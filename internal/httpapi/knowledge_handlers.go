package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/idgen"
	"agentcollab.local/agentcollab/internal/storage"
)

type KnowledgeRepository interface {
	CreateKnowledgeAsset(context.Context, domain.KnowledgeAsset, domain.UserID) (domain.KnowledgeAsset, error)
	ListWorkspaceKnowledge(context.Context, domain.WorkspaceID, domain.UserID) ([]domain.KnowledgeAsset, error)
	ListRoomKnowledge(context.Context, domain.RoomID, domain.UserID) ([]domain.KnowledgeAsset, error)
	KnowledgeAssetForUser(context.Context, domain.KnowledgeAssetID, domain.UserID) (domain.KnowledgeAsset, error)
	DeleteKnowledgeAsset(context.Context, domain.KnowledgeAssetID, domain.UserID) (domain.KnowledgeAsset, error)
	KnowledgeAssetForRunnerTask(context.Context, domain.TaskID, domain.KnowledgeAssetID, domain.RunnerID) (domain.KnowledgeAsset, error)
	ListProjectFileScopes(context.Context) ([]domain.ProjectFileScope, error)
	WorkspaceForRoomUser(context.Context, domain.RoomID, domain.UserID) (domain.WorkspaceID, error)
	AuthenticateRunner(context.Context, string) (domain.RunnerIdentity, error)
}

type KnowledgeHandlers struct {
	auth        *auth.Service
	repository  KnowledgeRepository
	storageRoot string
}

func NewKnowledgeHandlers(authService *auth.Service, repository KnowledgeRepository, storageRoot string) *KnowledgeHandlers {
	return &KnowledgeHandlers{auth: authService, repository: repository, storageRoot: storageRoot}
}

func (h *KnowledgeHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/knowledge", h.listWorkspace)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/knowledge", h.uploadWorkspace)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/knowledge/{assetID}", h.downloadWorkspace)
	mux.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/knowledge/{assetID}", h.deleteWorkspace)
	mux.HandleFunc("GET /api/v1/rooms/{roomID}/knowledge", h.listRoom)
	mux.HandleFunc("POST /api/v1/rooms/{roomID}/knowledge", h.uploadRoom)
	mux.HandleFunc("GET /api/v1/rooms/{roomID}/knowledge/{assetID}", h.downloadRoom)
	mux.HandleFunc("DELETE /api/v1/rooms/{roomID}/knowledge/{assetID}", h.deleteRoom)
	mux.HandleFunc("GET /api/v1/runner/tasks/{taskID}/knowledge/{assetID}", h.downloadForRunner)
}

func (h *KnowledgeHandlers) Reconcile(ctx context.Context) error {
	scopes, err := h.repository.ListProjectFileScopes(ctx)
	if err != nil {
		return err
	}
	for _, scope := range scopes {
		if scope.RoomID == nil {
			if err := storage.EnsureWorkspaceTree(h.storageRoot, string(scope.WorkspaceID)); err != nil {
				return err
			}
			continue
		}
		if err := storage.EnsureRoomTree(h.storageRoot, string(scope.WorkspaceID), string(*scope.RoomID)); err != nil {
			return err
		}
	}
	return nil
}

func (h *KnowledgeHandlers) listWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	workspaceID := domain.WorkspaceID(r.PathValue("workspaceID"))
	assets, err := h.repository.ListWorkspaceKnowledge(r.Context(), workspaceID, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	if err := storage.EnsureWorkspaceTree(h.storageRoot, string(workspaceID)); err != nil {
		writeError(w, http.StatusInternalServerError, "storage_unavailable", "无法准备工作区知识库目录")
		return
	}
	directory, _ := storage.WorkspaceDirectory(string(workspaceID))
	writeJSON(w, http.StatusOK, map[string]any{"assets": assets, "directory": filepath.Join(directory, "knowledge")})
}

func (h *KnowledgeHandlers) listRoom(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	roomID := domain.RoomID(r.PathValue("roomID"))
	assets, err := h.repository.ListRoomKnowledge(r.Context(), roomID, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	workspaceID, err := h.repository.WorkspaceForRoomUser(r.Context(), roomID, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	if err := storage.EnsureRoomTree(h.storageRoot, string(workspaceID), string(roomID)); err != nil {
		writeError(w, http.StatusInternalServerError, "storage_unavailable", "无法准备群聊知识库目录")
		return
	}
	directory, _ := storage.RoomDirectory(string(workspaceID), string(roomID))
	writeJSON(w, http.StatusOK, map[string]any{"assets": assets, "directory": filepath.Join(directory, "knowledge")})
}

func (h *KnowledgeHandlers) uploadWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	workspaceID := domain.WorkspaceID(r.PathValue("workspaceID"))
	h.upload(w, r, user, workspaceID, nil)
}

func (h *KnowledgeHandlers) uploadRoom(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	roomID := domain.RoomID(r.PathValue("roomID"))
	workspaceID, err := h.repository.WorkspaceForRoomUser(r.Context(), roomID, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	h.upload(w, r, user, workspaceID, &roomID)
}

func (h *KnowledgeHandlers) upload(w http.ResponseWriter, r *http.Request, user domain.User, workspaceID domain.WorkspaceID, roomID *domain.RoomID) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentBytes+(1<<20))
	if err := r.ParseMultipartForm(maxAttachmentBytes); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "asset_too_large", "单个资料不能超过 50 MB")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	source, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "asset_required", "请选择要上传的资料")
		return
	}
	defer source.Close()
	filename, err := safeAttachmentFilename(header.Filename)
	if err != nil {
		writeError(w, http.StatusBadRequest, "asset_type_not_allowed", err.Error())
		return
	}
	assetID, err := idgen.New("knowledge")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "无法创建知识资产")
		return
	}
	var relativeBase string
	if roomID == nil {
		if err := storage.EnsureWorkspaceTree(h.storageRoot, string(workspaceID)); err != nil {
			writeError(w, http.StatusInternalServerError, "storage_unavailable", "无法准备工作区知识库目录")
			return
		}
		workspaceDir, _ := storage.WorkspaceDirectory(string(workspaceID))
		relativeBase = filepath.Join(workspaceDir, "knowledge", assetID)
	} else {
		if err := storage.EnsureRoomTree(h.storageRoot, string(workspaceID), string(*roomID)); err != nil {
			writeError(w, http.StatusInternalServerError, "storage_unavailable", "无法准备群聊知识库目录")
			return
		}
		roomDir, _ := storage.RoomDirectory(string(workspaceID), string(*roomID))
		relativeBase = filepath.Join(roomDir, "knowledge", assetID)
	}
	absoluteDir, err := storage.ResolveWithin(h.storageRoot, relativeBase)
	if err != nil || os.MkdirAll(absoluteDir, 0o700) != nil {
		writeError(w, http.StatusInternalServerError, "storage_unavailable", "无法准备知识库目录")
		return
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(absoluteDir)
		}
	}()
	temporary, err := os.CreateTemp(absoluteDir, ".upload-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "无法保存资料")
		return
	}
	hasher := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(temporary, hasher), io.LimitReader(source, maxAttachmentBytes+1))
	closeErr := temporary.Close()
	if copyErr != nil || closeErr != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "资料上传失败")
		return
	}
	if size > maxAttachmentBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "asset_too_large", "单个资料不能超过 50 MB")
		return
	}
	finalPath := filepath.Join(absoluteDir, filename)
	if err := os.Chmod(temporary.Name(), 0o600); err != nil || os.Rename(temporary.Name(), finalPath) != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "无法完成资料保存")
		return
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if contentType == "" {
		contentType = strings.TrimSpace(header.Header.Get("Content-Type"))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	asset := domain.KnowledgeAsset{
		ID: domain.KnowledgeAssetID(assetID), WorkspaceID: workspaceID, RoomID: roomID, UploaderUserID: user.ID,
		UploaderName: user.DisplayName, Filename: filename, ContentType: contentType, SizeBytes: size,
		SHA256: hex.EncodeToString(hasher.Sum(nil)), RelativePath: filepath.Join(relativeBase, filename), CreatedAt: time.Now().UTC(),
	}
	asset, err = h.repository.CreateKnowledgeAsset(r.Context(), asset, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	cleanup = false
	writeJSON(w, http.StatusCreated, asset)
}

func (h *KnowledgeHandlers) downloadWorkspace(w http.ResponseWriter, r *http.Request) {
	h.downloadForUser(w, r, domain.WorkspaceID(r.PathValue("workspaceID")), nil)
}

func (h *KnowledgeHandlers) downloadRoom(w http.ResponseWriter, r *http.Request) {
	roomID := domain.RoomID(r.PathValue("roomID"))
	h.downloadForUser(w, r, "", &roomID)
}

func (h *KnowledgeHandlers) downloadForUser(w http.ResponseWriter, r *http.Request, workspaceID domain.WorkspaceID, roomID *domain.RoomID) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	asset, err := h.repository.KnowledgeAssetForUser(r.Context(), domain.KnowledgeAssetID(r.PathValue("assetID")), user.ID)
	if err != nil || (roomID == nil && (asset.RoomID != nil || asset.WorkspaceID != workspaceID)) || (roomID != nil && (asset.RoomID == nil || *asset.RoomID != *roomID)) {
		h.writeRepositoryError(w, auth.ErrNotFound)
		return
	}
	h.serveAsset(w, r, asset)
}

func (h *KnowledgeHandlers) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	h.deleteForUser(w, r, domain.WorkspaceID(r.PathValue("workspaceID")), nil)
}

func (h *KnowledgeHandlers) deleteRoom(w http.ResponseWriter, r *http.Request) {
	roomID := domain.RoomID(r.PathValue("roomID"))
	h.deleteForUser(w, r, "", &roomID)
}

func (h *KnowledgeHandlers) deleteForUser(w http.ResponseWriter, r *http.Request, workspaceID domain.WorkspaceID, roomID *domain.RoomID) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	asset, err := h.repository.KnowledgeAssetForUser(r.Context(), domain.KnowledgeAssetID(r.PathValue("assetID")), user.ID)
	if err != nil || (roomID == nil && (asset.RoomID != nil || asset.WorkspaceID != workspaceID)) || (roomID != nil && (asset.RoomID == nil || *asset.RoomID != *roomID)) {
		h.writeRepositoryError(w, auth.ErrNotFound)
		return
	}
	asset, err = h.repository.DeleteKnowledgeAsset(r.Context(), asset.ID, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	absolutePath, resolveErr := storage.ResolveWithin(h.storageRoot, asset.RelativePath)
	if resolveErr == nil {
		_ = os.RemoveAll(filepath.Dir(absolutePath))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *KnowledgeHandlers) downloadForRunner(w http.ResponseWriter, r *http.Request) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "runner bearer token is required")
		return
	}
	identity, err := h.repository.AuthenticateRunner(r.Context(), auth.HashToken(strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "runner bearer token is invalid")
		return
	}
	asset, err := h.repository.KnowledgeAssetForRunnerTask(r.Context(), domain.TaskID(r.PathValue("taskID")), domain.KnowledgeAssetID(r.PathValue("assetID")), identity.RunnerID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	h.serveAsset(w, r, asset)
}

func (h *KnowledgeHandlers) serveAsset(w http.ResponseWriter, r *http.Request, asset domain.KnowledgeAsset) {
	absolutePath, err := storage.ResolveWithin(h.storageRoot, asset.RelativePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资料不存在")
		return
	}
	file, err := os.Open(absolutePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "资料不存在")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != asset.SizeBytes {
		writeError(w, http.StatusNotFound, "not_found", "资料不可用")
		return
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": asset.Filename}))
	w.Header().Set("Content-Type", asset.ContentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-AgentCollab-SHA256", asset.SHA256)
	http.ServeContent(w, r, asset.Filename, asset.CreatedAt, file)
}

func (h *KnowledgeHandlers) authenticateUser(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "authentication is required")
		return domain.User{}, false
	}
	user, err := h.auth.Authenticate(r.Context(), cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "authentication is required")
		return domain.User{}, false
	}
	return user, true
}

func (h *KnowledgeHandlers) writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "the requested resource was not found")
	case errors.Is(err, auth.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "the requested action is not allowed")
	case errors.Is(err, auth.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "资料不可用")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "the request could not be completed")
	}
}
