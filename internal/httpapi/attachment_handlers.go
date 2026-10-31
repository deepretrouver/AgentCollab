package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/idgen"
	"agentcollab.local/agentcollab/internal/storage"
)

const maxAttachmentBytes int64 = 50 << 20

var allowedAttachmentExtensions = map[string]bool{
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".csv": true,
	".txt": true, ".md": true, ".ppt": true, ".pptx": true, ".png": true, ".jpg": true,
	".jpeg": true, ".webp": true,
	".zip": true, ".json": true,
}

type AttachmentRepository interface {
	CreateAttachment(context.Context, domain.Attachment, domain.UserID) (domain.Attachment, error)
	CreateTaskArtifact(context.Context, domain.TaskID, domain.RunnerID, domain.Attachment) (domain.Attachment, error)
	WorkspaceForRoomUser(context.Context, domain.RoomID, domain.UserID) (domain.WorkspaceID, error)
	AttachmentForUser(context.Context, domain.AttachmentID, domain.UserID) (domain.Attachment, error)
	AttachmentForRunnerTask(context.Context, domain.TaskID, domain.AttachmentID, domain.RunnerID) (domain.Attachment, error)
	AuthenticateRunner(context.Context, string) (domain.RunnerIdentity, error)
}

type AttachmentHandlers struct {
	auth        *auth.Service
	repository  AttachmentRepository
	storageRoot string
}

func NewAttachmentHandlers(authService *auth.Service, repository AttachmentRepository, storageRoot string) *AttachmentHandlers {
	return &AttachmentHandlers{auth: authService, repository: repository, storageRoot: storageRoot}
}

func (h *AttachmentHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/rooms/{roomID}/attachments", h.upload)
	mux.HandleFunc("POST /api/v1/runner/tasks/{taskID}/artifacts", h.uploadTaskArtifact)
	mux.HandleFunc("GET /api/v1/rooms/{roomID}/attachments/{attachmentID}", h.downloadForUser)
	mux.HandleFunc("GET /api/v1/runner/tasks/{taskID}/attachments/{attachmentID}", h.downloadForRunner)
}

func (h *AttachmentHandlers) uploadTaskArtifact(w http.ResponseWriter, r *http.Request) {
	headerValue := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(headerValue, "Bearer ") {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "runner bearer token is required")
		return
	}
	identity, err := h.repository.AuthenticateRunner(r.Context(), auth.HashToken(strings.TrimSpace(strings.TrimPrefix(headerValue, "Bearer "))))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "runner bearer token is invalid")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentBytes+(1<<20))
	if err := r.ParseMultipartForm(maxAttachmentBytes); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "attachment_too_large", "单个交付文件不能超过 50 MB")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	source, fileHeader, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "attachment_required", "请选择要交付的文件")
		return
	}
	defer source.Close()
	filename, err := safeAttachmentFilename(fileHeader.Filename)
	if err != nil {
		writeError(w, http.StatusBadRequest, "attachment_type_not_allowed", err.Error())
		return
	}
	attachmentID, err := idgen.New("attachment")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "无法创建交付文件")
		return
	}
	taskID := domain.TaskID(r.PathValue("taskID"))
	relativeDir := filepath.Join("artifacts", "tasks", string(taskID), attachmentID)
	absoluteDir, err := storage.ResolveWithin(h.storageRoot, relativeDir)
	if err != nil || os.MkdirAll(absoluteDir, 0o700) != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "无法准备交付文件存储")
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
		writeError(w, http.StatusInternalServerError, "internal_error", "无法保存交付文件")
		return
	}
	hasher := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(temporary, hasher), io.LimitReader(source, maxAttachmentBytes+1))
	closeErr := temporary.Close()
	if copyErr != nil || closeErr != nil || size > maxAttachmentBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "attachment_too_large", "单个交付文件不能超过 50 MB")
		return
	}
	finalPath := filepath.Join(absoluteDir, filename)
	if err := os.Chmod(temporary.Name(), 0o600); err != nil || os.Rename(temporary.Name(), finalPath) != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "无法完成交付文件保存")
		return
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	attachment := domain.Attachment{ID: domain.AttachmentID(attachmentID), Filename: filename, ContentType: contentType,
		SizeBytes: size, SHA256: hex.EncodeToString(hasher.Sum(nil)), RelativePath: filepath.Join(relativeDir, filename), CreatedAt: time.Now().UTC()}
	attachment, err = h.repository.CreateTaskArtifact(r.Context(), taskID, identity.RunnerID, attachment)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	cleanup = false
	writeJSON(w, http.StatusCreated, attachment)
}

func (h *AttachmentHandlers) upload(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentBytes+(1<<20))
	if err := r.ParseMultipartForm(maxAttachmentBytes); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "attachment_too_large", "单个附件不能超过 50 MB")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	source, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "attachment_required", "请选择要发送的文件")
		return
	}
	defer source.Close()
	filename, err := safeAttachmentFilename(header.Filename)
	if err != nil {
		writeError(w, http.StatusBadRequest, "attachment_type_not_allowed", err.Error())
		return
	}
	attachmentID, err := idgen.New("attachment")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "无法创建附件")
		return
	}
	roomID := domain.RoomID(r.PathValue("roomID"))
	workspaceID, err := h.repository.WorkspaceForRoomUser(r.Context(), roomID, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	if err := storage.EnsureRoomTree(h.storageRoot, string(workspaceID), string(roomID)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "无法准备附件存储")
		return
	}
	roomDir, err := storage.RoomDirectory(string(workspaceID), string(roomID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "无法准备附件存储")
		return
	}
	relativeDir := filepath.Join(roomDir, "uploads", attachmentID)
	absoluteDir, err := storage.ResolveWithin(h.storageRoot, relativeDir)
	if err != nil || os.MkdirAll(absoluteDir, 0o700) != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "无法准备附件存储")
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
		writeError(w, http.StatusInternalServerError, "internal_error", "无法保存附件")
		return
	}
	temporaryPath := temporary.Name()
	hasher := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(temporary, hasher), io.LimitReader(source, maxAttachmentBytes+1))
	closeErr := temporary.Close()
	if copyErr != nil || closeErr != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "附件上传失败")
		return
	}
	if size > maxAttachmentBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "attachment_too_large", "单个附件不能超过 50 MB")
		return
	}
	finalPath := filepath.Join(absoluteDir, filename)
	if err := os.Chmod(temporaryPath, 0o600); err != nil || os.Rename(temporaryPath, finalPath) != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "无法完成附件保存")
		return
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if contentType == "" {
		contentType = strings.TrimSpace(header.Header.Get("Content-Type"))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	attachment := domain.Attachment{
		ID: domain.AttachmentID(attachmentID), WorkspaceID: workspaceID, RoomID: roomID, UploaderUserID: user.ID,
		Filename: filename, ContentType: contentType, SizeBytes: size, SHA256: hex.EncodeToString(hasher.Sum(nil)),
		RelativePath: filepath.Join(relativeDir, filename), CreatedAt: time.Now().UTC(),
	}
	attachment, err = h.repository.CreateAttachment(r.Context(), attachment, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	cleanup = false
	writeJSON(w, http.StatusCreated, attachment)
}

func (h *AttachmentHandlers) downloadForUser(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	attachment, err := h.repository.AttachmentForUser(r.Context(), domain.AttachmentID(r.PathValue("attachmentID")), user.ID)
	if err != nil || attachment.RoomID != domain.RoomID(r.PathValue("roomID")) {
		h.writeRepositoryError(w, auth.ErrNotFound)
		return
	}
	h.serveAttachment(w, r, attachment)
}

func (h *AttachmentHandlers) downloadForRunner(w http.ResponseWriter, r *http.Request) {
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
	attachment, err := h.repository.AttachmentForRunnerTask(r.Context(), domain.TaskID(r.PathValue("taskID")), domain.AttachmentID(r.PathValue("attachmentID")), identity.RunnerID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	h.serveAttachment(w, r, attachment)
}

func (h *AttachmentHandlers) serveAttachment(w http.ResponseWriter, r *http.Request, attachment domain.Attachment) {
	absolutePath, err := storage.ResolveWithin(h.storageRoot, attachment.RelativePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "附件不存在")
		return
	}
	file, err := os.Open(absolutePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "附件不存在")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != attachment.SizeBytes {
		writeError(w, http.StatusNotFound, "not_found", "附件不可用")
		return
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": attachment.Filename})
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Content-Type", attachment.ContentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-AgentCollab-SHA256", attachment.SHA256)
	http.ServeContent(w, r, attachment.Filename, attachment.CreatedAt, file)
}

func (h *AttachmentHandlers) authenticateUser(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
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

func (h *AttachmentHandlers) writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "the requested resource was not found")
	case errors.Is(err, auth.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "the requested action is not allowed")
	case errors.Is(err, auth.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "任务已经结束，不能继续追加交付文件")
	case errors.Is(err, auth.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "附件不可用或不属于当前消息发送者")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "the request could not be completed")
	}
}

func safeAttachmentFilename(value string) (string, error) {
	value = strings.ReplaceAll(value, "\\", "/")
	value = filepath.Base(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if r == 0 || r == '/' || r == '\\' || unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	if value == "" || value == "." || len([]byte(value)) > 240 {
		return "", fmt.Errorf("文件名无效")
	}
	if !allowedAttachmentExtensions[strings.ToLower(filepath.Ext(value))] {
		return "", fmt.Errorf("当前支持 PDF、Word、Excel、CSV、TXT、Markdown、PPT、ZIP、JSON 和常见图片")
	}
	return value, nil
}
