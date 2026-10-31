package httpapi

import (
	"net/http"
	"os"
	"strings"
	"time"

	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/idgen"
	"agentcollab.local/agentcollab/internal/storage"
)

func (h *CollabHandlers) listRooms(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	rooms, err := h.repository.ListRooms(r.Context(), domain.WorkspaceID(r.PathValue("workspaceID")), user.ID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": rooms})
}

func (h *CollabHandlers) createRoom(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_request", "room name must be 1-80 characters")
		return
	}
	id, err := idgen.New("room")
	if err != nil {
		h.writeError(w, err)
		return
	}
	room := domain.Room{ID: domain.RoomID(id), WorkspaceID: domain.WorkspaceID(r.PathValue("workspaceID")), Name: input.Name, CreatedAt: time.Now().UTC()}
	if h.storageRoot != "" {
		if err := storage.EnsureRoomTree(h.storageRoot, string(room.WorkspaceID), id); err != nil {
			writeError(w, http.StatusInternalServerError, "storage_unavailable", "无法创建群聊文件目录")
			return
		}
	}
	if err := h.repository.CreateRoom(r.Context(), room, user.ID); err != nil {
		if h.storageRoot != "" {
			if relative, pathErr := storage.RoomDirectory(string(room.WorkspaceID), id); pathErr == nil {
				if absolute, pathErr := storage.ResolveWithin(h.storageRoot, relative); pathErr == nil {
					_ = os.RemoveAll(absolute)
				}
			}
		}
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, room)
}

func (h *CollabHandlers) setRoomPreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var input struct {
		Pinned bool `json:"pinned"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	room, err := h.repository.SetRoomPinned(r.Context(), domain.RoomID(r.PathValue("roomID")), user.ID, input.Pinned)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, room)
}

func (h *CollabHandlers) addRoomMember(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var input struct {
		UserID domain.UserID `json:"user_id"`
	}
	if err := decodeJSON(w, r, &input); err != nil || input.UserID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "user_id is required")
		return
	}
	if err := h.repository.AddRoomMember(r.Context(), domain.RoomID(r.PathValue("roomID")), user.ID, input.UserID); err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"room_id": r.PathValue("roomID"), "user_id": input.UserID})
}

func (h *CollabHandlers) listRoomMembers(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	users, err := h.repository.ListRoomMembers(r.Context(), domain.RoomID(r.PathValue("roomID")), user.ID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	members := make([]map[string]any, 0, len(users))
	for _, member := range users {
		members = append(members, userResponse(member))
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

func (h *CollabHandlers) updateRoomMember(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var input struct {
		Role domain.RoomRole `json:"role"`
	}
	if err := decodeJSON(w, r, &input); err != nil || (input.Role != domain.RoomAdmin && input.Role != domain.RoomMember) {
		writeError(w, http.StatusBadRequest, "invalid_request", "role must be admin or member")
		return
	}
	if err := h.repository.UpdateRoomMemberRole(r.Context(), domain.RoomID(r.PathValue("roomID")), domain.UserID(r.PathValue("userID")), input.Role, user.ID); err != nil {
		h.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *CollabHandlers) removeRoomMember(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if err := h.repository.RemoveRoomMember(r.Context(), domain.RoomID(r.PathValue("roomID")), domain.UserID(r.PathValue("userID")), user.ID); err != nil {
		h.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
