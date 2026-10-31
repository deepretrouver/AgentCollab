package httpapi

import (
	"errors"
	"net/http"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
)

type AuthHandlers struct {
	service          *auth.Service
	runnerPublicURLs []string
}

func NewAuthHandlers(service *auth.Service) *AuthHandlers {
	return &AuthHandlers{service: service}
}

func NewAuthHandlersWithRunnerURLs(service *auth.Service, runnerPublicURLs []string) *AuthHandlers {
	return &AuthHandlers{service: service, runnerPublicURLs: append([]string{}, runnerPublicURLs...)}
}

func (h *AuthHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/login", h.login)
	mux.HandleFunc("POST /api/v1/auth/register", h.register)
	mux.HandleFunc("POST /api/v1/auth/logout", h.logout)
	mux.HandleFunc("GET /api/v1/auth/me", h.me)
	mux.HandleFunc("PATCH /api/v1/auth/me", h.updateProfile)
	mux.HandleFunc("GET /api/v1/admin/users", h.listHubUsers)
	mux.HandleFunc("PATCH /api/v1/admin/users/{userID}", h.updateHubUserRole)
	mux.HandleFunc("DELETE /api/v1/admin/users/{userID}", h.archiveHubUser)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/invites", h.createInvite)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/runner-invites", h.createRunnerInvite)
	mux.HandleFunc("POST /api/v1/invites/{code}/register", h.registerWithInvite)
	mux.HandleFunc("POST /api/v1/invites/{code}/accept", h.acceptInvite)
}

func (h *AuthHandlers) register(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	login, err := h.service.Register(r.Context(), input.Username, input.DisplayName, input.Password)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.setSessionCookie(w, r, login)
	writeJSON(w, http.StatusCreated, userResponse(login.User))
}

func (h *AuthHandlers) createRunnerInvite(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var input struct {
		ExpiresInSeconds int64 `json:"expires_in_seconds"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if input.ExpiresInSeconds == 0 {
		input.ExpiresInSeconds = 30 * 60
	}
	result, err := h.service.CreateRunnerInvite(r.Context(), user.ID, domain.WorkspaceID(r.PathValue("workspaceID")), time.Duration(input.ExpiresInSeconds)*time.Second)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":            result.Invite.ID,
		"code":          result.Code,
		"workspace_id":  result.Invite.WorkspaceID,
		"owner_user_id": result.Invite.OwnerUserID,
		"expires_at":    result.Invite.ExpiresAt,
		"hub_urls":      h.runnerPublicURLs,
	})
}

func (h *AuthHandlers) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := h.service.Login(r.Context(), input.Username, input.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "username or password is invalid")
		} else {
			writeError(w, http.StatusInternalServerError, "internal_error", "authentication service is unavailable")
		}
		return
	}
	h.setSessionCookie(w, r, result)
	writeJSON(w, http.StatusOK, userResponse(result.User))
}

func (h *AuthHandlers) setSessionCookie(w http.ResponseWriter, r *http.Request, result auth.LoginResult) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    result.Token,
		Path:     "/",
		Expires:  result.ExpiresAt,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandlers) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil || h.service.Logout(r.Context(), cookie.Value) != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "authentication is required")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandlers) me(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, userResponse(user))
}

func (h *AuthHandlers) updateProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var input auth.ProfileInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	updated, err := h.service.UpdateProfile(r.Context(), user.ID, input)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userResponse(updated))
}

func (h *AuthHandlers) listHubUsers(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	users, err := h.service.ListHubUsers(r.Context(), user.ID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	response := make([]map[string]any, 0, len(users))
	for _, account := range users {
		response = append(response, userResponse(account))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": response})
}

func (h *AuthHandlers) updateHubUserRole(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var input struct {
		Role domain.UserRole `json:"role"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	updated, err := h.service.UpdateHubUserRole(r.Context(), user.ID, domain.UserID(r.PathValue("userID")), input.Role)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userResponse(updated))
}

func (h *AuthHandlers) archiveHubUser(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if err := h.service.ArchiveHubUser(r.Context(), user.ID, domain.UserID(r.PathValue("userID"))); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandlers) createInvite(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var input struct {
		Role             domain.WorkspaceRole `json:"role"`
		ExpiresInSeconds int64                `json:"expires_in_seconds"`
		RoomID           string               `json:"room_id"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := h.service.CreateInvite(r.Context(), user.ID, domain.WorkspaceID(r.PathValue("workspaceID")), input.Role, time.Duration(input.ExpiresInSeconds)*time.Second, domain.RoomID(input.RoomID))
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":           result.Invite.ID,
		"code":         result.Code,
		"workspace_id": result.Invite.WorkspaceID,
		"room_id":      result.Invite.RoomID,
		"role":         result.Invite.Role,
		"expires_at":   result.Invite.ExpiresAt,
		"hub_urls":     h.runnerPublicURLs,
	})
}

func (h *AuthHandlers) registerWithInvite(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	login, membership, err := h.service.RegisterWithInvite(r.Context(), r.PathValue("code"), input.Username, input.DisplayName, input.Password)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	h.setSessionCookie(w, r, login)
	writeJSON(w, http.StatusCreated, map[string]any{
		"user":         userResponse(login.User),
		"workspace_id": membership.WorkspaceID,
		"role":         membership.Role,
	})
}

func (h *AuthHandlers) acceptInvite(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	membership, err := h.service.AcceptInvite(r.Context(), r.PathValue("code"), user.ID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_id": membership.WorkspaceID,
		"user_id":      membership.UserID,
		"role":         membership.Role,
		"joined_at":    membership.JoinedAt,
	})
}

func (h *AuthHandlers) authenticate(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "authentication is required")
		return domain.User{}, false
	}
	user, err := h.service.Authenticate(r.Context(), cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "authentication is required")
		return domain.User{}, false
	}
	return user, true
}

func (h *AuthHandlers) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrDefaultHubAdmin):
		writeError(w, http.StatusConflict, "default_hub_admin", "默认 owner 账号必须保留 Hub 管理员权限。")
	case errors.Is(err, auth.ErrLastHubAdmin):
		writeError(w, http.StatusConflict, "last_hub_admin", "至少需要保留一个 Hub 管理员，无法删除该账号。")
	case errors.Is(err, auth.ErrAccountOwnsWorkspace):
		writeError(w, http.StatusConflict, "account_owns_workspace", "该账号仍是工作区 Owner。请先转移或删除其工作区，再删除账号。")
	case errors.Is(err, auth.ErrAccountHasInfrastructure):
		writeError(w, http.StatusConflict, "account_has_infrastructure", "该账号仍拥有已接入的本机 Agent 或设备。请先迁移或移除这些资源，再删除账号。")
	case errors.Is(err, auth.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "the requested action is not allowed")
	case errors.Is(err, auth.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "the requested account was not found")
	case errors.Is(err, auth.ErrInviteInvalid), errors.Is(err, auth.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, auth.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "the request could not be completed")
	}
}

func userResponse(user domain.User) map[string]any {
	result := map[string]any{
		"id":            user.ID,
		"username":      user.Username,
		"display_name":  user.DisplayName,
		"avatar_url":    user.AvatarURL,
		"bio":           user.Bio,
		"current_focus": user.CurrentFocus,
		"role":          user.Role,
	}
	if user.WorkspaceRole != "" {
		result["workspace_role"] = user.WorkspaceRole
	}
	if user.RoomRole != "" {
		result["room_role"] = user.RoomRole
	}
	return result
}
