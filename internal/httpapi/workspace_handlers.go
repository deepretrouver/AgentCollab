package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/cgi"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/events"
	"agentcollab.local/agentcollab/internal/idgen"
	"agentcollab.local/agentcollab/internal/storage"
)

type CollabRepository interface {
	CreateWorkspace(context.Context, domain.Workspace, domain.UserID) error
	ListWorkspaces(context.Context, domain.UserID) ([]domain.Workspace, error)
	UpdateWorkspace(context.Context, domain.WorkspaceID, string, domain.UserID) (domain.Workspace, error)
	DeleteWorkspace(context.Context, domain.WorkspaceID, domain.UserID) error
	ListWorkspaceMembers(context.Context, domain.WorkspaceID, domain.UserID) ([]domain.User, error)
	UpdateWorkspaceMemberRole(context.Context, domain.WorkspaceID, domain.UserID, domain.WorkspaceRole, domain.UserID) error
	RemoveWorkspaceMember(context.Context, domain.WorkspaceID, domain.UserID, domain.UserID) error
	CreateRoom(context.Context, domain.Room, domain.UserID) error
	ListRooms(context.Context, domain.WorkspaceID, domain.UserID) ([]domain.Room, error)
	SetRoomPinned(context.Context, domain.RoomID, domain.UserID, bool) (domain.Room, error)
	AddRoomMember(context.Context, domain.RoomID, domain.UserID, domain.UserID) error
	UpdateRoomMemberRole(context.Context, domain.RoomID, domain.UserID, domain.RoomRole, domain.UserID) error
	RemoveRoomMember(context.Context, domain.RoomID, domain.UserID, domain.UserID) error
	ListRoomMembers(context.Context, domain.RoomID, domain.UserID) ([]domain.User, error)
	CreateMessage(context.Context, domain.Message, domain.UserID) (domain.Message, error)
	ListMessages(context.Context, domain.RoomID, domain.UserID, uint64, int) ([]domain.Message, error)
}

type CollabHandlers struct {
	auth        *auth.Service
	repository  CollabRepository
	publisher   EventPublisher
	storageRoot string
}

func (h *CollabHandlers) WithStorageRoot(root string) *CollabHandlers {
	h.storageRoot = root
	return h
}

type EventPublisher interface {
	Publish(events.Envelope)
}

func NewCollabHandlers(authService *auth.Service, repository CollabRepository, publishers ...EventPublisher) *CollabHandlers {
	handler := &CollabHandlers{auth: authService, repository: repository}
	if len(publishers) > 0 {
		handler.publisher = publishers[0]
	}
	return handler
}

func (h *CollabHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workspaces", h.listWorkspaces)
	mux.HandleFunc("POST /api/v1/workspaces", h.createWorkspace)
	mux.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}", h.updateWorkspace)
	mux.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}", h.deleteWorkspace)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/repository", h.workspaceRepository)
	mux.HandleFunc("/git/{workspaceID}/{path...}", h.serveWorkspaceGit)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/members", h.listWorkspaceMembers)
	mux.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}/members/{userID}", h.updateWorkspaceMember)
	mux.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/members/{userID}", h.removeWorkspaceMember)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/rooms", h.listRooms)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/rooms", h.createRoom)
	mux.HandleFunc("PATCH /api/v1/rooms/{roomID}/preferences", h.setRoomPreferences)
	mux.HandleFunc("GET /api/v1/rooms/{roomID}/members", h.listRoomMembers)
	mux.HandleFunc("POST /api/v1/rooms/{roomID}/members", h.addRoomMember)
	mux.HandleFunc("PATCH /api/v1/rooms/{roomID}/members/{userID}", h.updateRoomMember)
	mux.HandleFunc("DELETE /api/v1/rooms/{roomID}/members/{userID}", h.removeRoomMember)
	mux.HandleFunc("GET /api/v1/rooms/{roomID}/messages", h.listMessages)
	mux.HandleFunc("POST /api/v1/rooms/{roomID}/messages", h.createMessage)
	mux.HandleFunc("GET /api/v1/rooms/{roomID}/timeline", h.timeline)
}

func (h *CollabHandlers) workspaceForUser(ctx context.Context, workspaceID domain.WorkspaceID, userID domain.UserID) (domain.Workspace, error) {
	workspaces, err := h.repository.ListWorkspaces(ctx, userID)
	if err != nil {
		return domain.Workspace{}, err
	}
	for _, workspace := range workspaces {
		if workspace.ID == workspaceID {
			return workspace, nil
		}
	}
	return domain.Workspace{}, auth.ErrNotFound
}

func (h *CollabHandlers) workspaceRepository(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	workspaceID := domain.WorkspaceID(r.PathValue("workspaceID"))
	if _, err := h.workspaceForUser(r.Context(), workspaceID, user.ID); err != nil {
		h.writeError(w, err)
		return
	}
	if h.storageRoot == "" {
		writeError(w, http.StatusServiceUnavailable, "repository_unavailable", "Server 尚未配置项目存储目录")
		return
	}
	if err := storage.EnsureWorkspaceGitRepository(h.storageRoot, string(workspaceID)); err != nil {
		writeError(w, http.StatusInternalServerError, "repository_unavailable", "无法初始化工作区 Git 主仓")
		return
	}
	info, err := storage.WorkspaceGitInfo(h.storageRoot, string(workspaceID))
	if err != nil {
		h.writeError(w, err)
		return
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"initialized": info.Initialized, "default_branch": info.DefaultBranch,
		"storage_path": info.StoragePath, "transport": info.Transport,
		"clone_url":      scheme + "://" + r.Host + "/git/" + string(workspaceID) + "/project.git",
		"main_protected": true,
	})
}

func (h *CollabHandlers) serveWorkspaceGit(w http.ResponseWriter, r *http.Request) {
	if h.storageRoot == "" {
		h.gitAuthenticationRequired(w)
		return
	}
	username, password, ok := r.BasicAuth()
	if !ok {
		h.gitAuthenticationRequired(w)
		return
	}
	user, err := h.auth.VerifyCredentials(r.Context(), username, password)
	if err != nil {
		h.gitAuthenticationRequired(w)
		return
	}
	workspaceID := domain.WorkspaceID(r.PathValue("workspaceID"))
	if _, err := h.workspaceForUser(r.Context(), workspaceID, user.ID); err != nil {
		h.gitAuthenticationRequired(w)
		return
	}
	if err := storage.EnsureWorkspaceGitRepository(h.storageRoot, string(workspaceID)); err != nil {
		writeError(w, http.StatusInternalServerError, "repository_unavailable", "Git 主仓暂不可用")
		return
	}
	git, err := exec.LookPath("git")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "repository_unavailable", "Server 未安装 Git")
		return
	}
	workspaceDir, _ := storage.WorkspaceDirectory(string(workspaceID))
	projectRoot, err := storage.ResolveWithin(h.storageRoot, filepath.Join(workspaceDir, "git"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "repository_unavailable", "Git 主仓路径无效")
		return
	}
	prefix := "/git/" + string(workspaceID)
	handler := &cgi.Handler{
		Path: git, Args: []string{"http-backend"}, Root: prefix,
		Dir: projectRoot,
		Env: []string{"GIT_PROJECT_ROOT=" + projectRoot, "GIT_HTTP_EXPORT_ALL=1", "REMOTE_USER=" + user.Username},
	}
	handler.ServeHTTP(w, r)
}

func (h *CollabHandlers) gitAuthenticationRequired(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="AgentCollab workspace Git"`)
	writeError(w, http.StatusUnauthorized, "git_authentication_required", "请使用 AgentCollab 用户名和密码访问工作区 Git")
}

func (h *CollabHandlers) listWorkspaceMembers(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	members, err := h.repository.ListWorkspaceMembers(r.Context(), domain.WorkspaceID(r.PathValue("workspaceID")), user.ID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response := make([]map[string]any, 0, len(members))
	for _, member := range members {
		response = append(response, userResponse(member))
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": response})
}

func (h *CollabHandlers) updateWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var input struct {
		Role domain.WorkspaceRole `json:"role"`
	}
	if err := decodeJSON(w, r, &input); err != nil || (input.Role != domain.WorkspaceAdmin && input.Role != domain.WorkspaceMember) {
		writeError(w, http.StatusBadRequest, "invalid_request", "role must be admin or member")
		return
	}
	if err := h.repository.UpdateWorkspaceMemberRole(r.Context(), domain.WorkspaceID(r.PathValue("workspaceID")), domain.UserID(r.PathValue("userID")), input.Role, user.ID); err != nil {
		h.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *CollabHandlers) removeWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if err := h.repository.RemoveWorkspaceMember(r.Context(), domain.WorkspaceID(r.PathValue("workspaceID")), domain.UserID(r.PathValue("userID")), user.ID); err != nil {
		h.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *CollabHandlers) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	workspaces, err := h.repository.ListWorkspaces(r.Context(), user.ID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": workspaces})
}

func (h *CollabHandlers) createWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 80 {
		writeError(w, http.StatusBadRequest, "invalid_request", "workspace name must be 1-80 characters")
		return
	}
	id, err := idgen.New("ws")
	if err != nil {
		h.writeError(w, err)
		return
	}
	now := time.Now().UTC()
	workspace := domain.Workspace{ID: domain.WorkspaceID(id), Name: input.Name, OwnerUserID: user.ID, Role: domain.WorkspaceOwner, CreatedAt: now, UpdatedAt: now}
	if h.storageRoot != "" {
		if err := storage.EnsureWorkspaceGitRepository(h.storageRoot, id); err != nil {
			writeError(w, http.StatusInternalServerError, "storage_unavailable", "无法创建工作区文件目录")
			return
		}
	}
	if err := h.repository.CreateWorkspace(r.Context(), workspace, user.ID); err != nil {
		if h.storageRoot != "" {
			if relative, pathErr := storage.WorkspaceDirectory(id); pathErr == nil {
				if absolute, pathErr := storage.ResolveWithin(h.storageRoot, relative); pathErr == nil {
					_ = os.RemoveAll(absolute)
				}
			}
		}
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, workspace)
}

func (h *CollabHandlers) updateWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 80 {
		writeError(w, http.StatusBadRequest, "invalid_request", "workspace name must be 1-80 characters")
		return
	}
	workspace, err := h.repository.UpdateWorkspace(r.Context(), domain.WorkspaceID(r.PathValue("workspaceID")), input.Name, user.ID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

func (h *CollabHandlers) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if err := h.repository.DeleteWorkspace(r.Context(), domain.WorkspaceID(r.PathValue("workspaceID")), user.ID); err != nil {
		h.writeError(w, err)
		return
	}
	if h.storageRoot != "" {
		if relative, err := storage.WorkspaceDirectory(r.PathValue("workspaceID")); err == nil {
			if absolute, err := storage.ResolveWithin(h.storageRoot, relative); err == nil {
				_ = os.RemoveAll(absolute)
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *CollabHandlers) authenticate(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
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

func (h *CollabHandlers) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "the requested resource was not found")
	case errors.Is(err, auth.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "the requested action is not allowed")
	case errors.Is(err, auth.ErrWorkspaceHasActiveTasks):
		writeError(w, http.StatusConflict, "workspace_has_active_tasks", "无法删除工作区：仍有排队或执行中的 Agent 任务。请先停止或等待任务结束。")
	case errors.Is(err, auth.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "the resource already exists")
	case errors.Is(err, auth.ErrCoordinatorRequired):
		writeError(w, http.StatusConflict, "coordinator_required", "请先在右侧将一个 Agent 设置为“主协调 / 自动分诊”")
	case errors.Is(err, auth.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "the request could not be completed")
	}
}
