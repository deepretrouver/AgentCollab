package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/httpapi"
	store "agentcollab.local/agentcollab/internal/store/sqlite"
)

func TestWorkspaceRoomAndMessageAPI(t *testing.T) {
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	authService := auth.NewService(store.NewAuthRepository(database))
	owner, err := authService.CreateUser(context.Background(), "owner", "Owner", "password-1234", domain.UserAdmin)
	if err != nil {
		t.Fatal(err)
	}
	member, err := authService.CreateUser(context.Background(), "member", "Member", "password-5678", domain.UserMember)
	if err != nil {
		t.Fatal(err)
	}
	_, err = authService.CreateUser(context.Background(), "outsider", "Outsider", "password-9012", domain.UserMember)
	if err != nil {
		t.Fatal(err)
	}

	repository := store.NewCollabRepository(database)
	handler := httpapi.NewRouter(authService, repository)
	ownerCookie := loginCookie(t, handler, "owner", "password-1234")
	memberCookie := loginCookie(t, handler, "member", "password-5678")
	outsiderCookie := loginCookie(t, handler, "outsider", "password-9012")

	workspaceResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces", map[string]any{"name": "Research"}, ownerCookie)
	if workspaceResponse.Code != http.StatusCreated {
		t.Fatalf("create workspace status=%d body=%s", workspaceResponse.Code, workspaceResponse.Body.String())
	}
	var workspace struct {
		ID string `json:"id"`
	}
	decodeResponse(t, workspaceResponse, &workspace)
	if workspace.ID == "" {
		t.Fatal("workspace id is empty")
	}
	renamedWorkspace := requestJSON(t, handler, http.MethodPatch, "/api/v1/workspaces/"+workspace.ID, map[string]any{"name": "Research Team"}, ownerCookie)
	if renamedWorkspace.Code != http.StatusOK || !bytes.Contains(renamedWorkspace.Body.Bytes(), []byte(`"name":"Research Team"`)) {
		t.Fatalf("owner rename status=%d body=%s", renamedWorkspace.Code, renamedWorkspace.Body.String())
	}
	invite, err := authService.CreateInvite(context.Background(), owner.ID, domain.WorkspaceID(workspace.ID), domain.WorkspaceMember, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authService.AcceptInvite(context.Background(), invite.Code, member.ID); err != nil {
		t.Fatal(err)
	}
	forbiddenRename := requestJSON(t, handler, http.MethodPatch, "/api/v1/workspaces/"+workspace.ID, map[string]any{"name": "Member cannot rename"}, memberCookie)
	if forbiddenRename.Code != http.StatusForbidden {
		t.Fatalf("member rename status=%d body=%s", forbiddenRename.Code, forbiddenRename.Body.String())
	}

	outsiderRooms := requestJSON(t, handler, http.MethodGet, "/api/v1/workspaces/"+workspace.ID+"/rooms", nil, outsiderCookie)
	if outsiderRooms.Code != http.StatusNotFound {
		t.Fatalf("outsider rooms status=%d body=%s", outsiderRooms.Code, outsiderRooms.Body.String())
	}

	roomResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces/"+workspace.ID+"/rooms", map[string]any{"name": "General"}, ownerCookie)
	if roomResponse.Code != http.StatusCreated {
		t.Fatalf("create room status=%d body=%s", roomResponse.Code, roomResponse.Body.String())
	}
	var room struct {
		ID string `json:"id"`
	}
	decodeResponse(t, roomResponse, &room)

	addMember := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+room.ID+"/members", map[string]any{"user_id": member.ID}, ownerCookie)
	if addMember.Code != http.StatusCreated {
		t.Fatalf("add room member status=%d body=%s", addMember.Code, addMember.Body.String())
	}
	roomMembers := requestJSON(t, handler, http.MethodGet, "/api/v1/rooms/"+room.ID+"/members", nil, ownerCookie)
	if roomMembers.Code != http.StatusOK || !bytes.Contains(roomMembers.Body.Bytes(), []byte(`"room_role":"owner"`)) || !bytes.Contains(roomMembers.Body.Bytes(), []byte(`"room_role":"member"`)) {
		t.Fatalf("room members roles status=%d body=%s", roomMembers.Code, roomMembers.Body.String())
	}
	selfPromote := requestJSON(t, handler, http.MethodPatch, "/api/v1/rooms/"+room.ID+"/members/"+string(member.ID), map[string]any{"role": "admin"}, memberCookie)
	if selfPromote.Code != http.StatusForbidden {
		t.Fatalf("room member self promotion status=%d body=%s", selfPromote.Code, selfPromote.Body.String())
	}
	promoteMember := requestJSON(t, handler, http.MethodPatch, "/api/v1/rooms/"+room.ID+"/members/"+string(member.ID), map[string]any{"role": "admin"}, ownerCookie)
	if promoteMember.Code != http.StatusNoContent {
		t.Fatalf("room owner promotion status=%d body=%s", promoteMember.Code, promoteMember.Body.String())
	}

	pinRoom := requestJSON(t, handler, http.MethodPatch, "/api/v1/rooms/"+room.ID+"/preferences", map[string]any{"pinned": true}, ownerCookie)
	if pinRoom.Code != http.StatusOK || !bytes.Contains(pinRoom.Body.Bytes(), []byte(`"pinned":true`)) || !bytes.Contains(pinRoom.Body.Bytes(), []byte(`"pinned_at"`)) {
		t.Fatalf("pin room status=%d body=%s", pinRoom.Code, pinRoom.Body.String())
	}
	ownerRooms := requestJSON(t, handler, http.MethodGet, "/api/v1/workspaces/"+workspace.ID+"/rooms", nil, ownerCookie)
	if ownerRooms.Code != http.StatusOK || !bytes.Contains(ownerRooms.Body.Bytes(), []byte(`"pinned":true`)) {
		t.Fatalf("owner rooms after pin status=%d body=%s", ownerRooms.Code, ownerRooms.Body.String())
	}
	memberRooms := requestJSON(t, handler, http.MethodGet, "/api/v1/workspaces/"+workspace.ID+"/rooms", nil, memberCookie)
	if memberRooms.Code != http.StatusOK || !bytes.Contains(memberRooms.Body.Bytes(), []byte(`"pinned":false`)) {
		t.Fatalf("member rooms should keep independent preferences status=%d body=%s", memberRooms.Code, memberRooms.Body.String())
	}

	messageResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+room.ID+"/messages", map[string]any{"body": "hello from member"}, memberCookie)
	if messageResponse.Code != http.StatusCreated {
		t.Fatalf("create message status=%d body=%s", messageResponse.Code, messageResponse.Body.String())
	}
	var firstMessage struct {
		Sequence uint64 `json:"sequence"`
		Kind     string `json:"kind"`
	}
	decodeResponse(t, messageResponse, &firstMessage)
	if firstMessage.Sequence != 1 || firstMessage.Kind != "chat" {
		t.Fatalf("first message=%#v", firstMessage)
	}

	executableChat := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+room.ID+"/messages", map[string]any{"body": "rm -rf /", "kind": "task"}, memberCookie)
	if executableChat.Code != http.StatusBadRequest {
		t.Fatalf("chat with task kind status=%d body=%s", executableChat.Code, executableChat.Body.String())
	}

	secondResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+room.ID+"/messages", map[string]any{"body": "second"}, memberCookie)
	if secondResponse.Code != http.StatusCreated {
		t.Fatalf("second message status=%d body=%s", secondResponse.Code, secondResponse.Body.String())
	}

	timeline := requestJSON(t, handler, http.MethodGet, "/api/v1/rooms/"+room.ID+"/timeline?after=1", nil, memberCookie)
	if timeline.Code != http.StatusOK {
		t.Fatalf("timeline status=%d body=%s", timeline.Code, timeline.Body.String())
	}
	var timelineBody struct {
		Messages []struct {
			Sequence uint64 `json:"sequence"`
			Body     string `json:"body"`
		} `json:"messages"`
	}
	decodeResponse(t, timeline, &timelineBody)
	if len(timelineBody.Messages) != 1 || timelineBody.Messages[0].Sequence != 2 || timelineBody.Messages[0].Body != "second" {
		t.Fatalf("timeline=%#v", timelineBody.Messages)
	}

	outsiderMessages := requestJSON(t, handler, http.MethodGet, "/api/v1/rooms/"+room.ID+"/messages", nil, outsiderCookie)
	if outsiderMessages.Code != http.StatusNotFound {
		t.Fatalf("outsider messages status=%d body=%s", outsiderMessages.Code, outsiderMessages.Body.String())
	}

	disposableResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces", map[string]any{"name": "Disposable"}, ownerCookie)
	if disposableResponse.Code != http.StatusCreated {
		t.Fatalf("create disposable workspace status=%d body=%s", disposableResponse.Code, disposableResponse.Body.String())
	}
	var disposable struct {
		ID string `json:"id"`
	}
	decodeResponse(t, disposableResponse, &disposable)
	forbiddenDelete := requestJSON(t, handler, http.MethodDelete, "/api/v1/workspaces/"+disposable.ID, nil, memberCookie)
	if forbiddenDelete.Code != http.StatusNotFound {
		t.Fatalf("nonmember delete status=%d body=%s", forbiddenDelete.Code, forbiddenDelete.Body.String())
	}
	deleted := requestJSON(t, handler, http.MethodDelete, "/api/v1/workspaces/"+disposable.ID, nil, ownerCookie)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("owner delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}

	protectedResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces", map[string]any{"name": "Protected Runner Home"}, ownerCookie)
	if protectedResponse.Code != http.StatusCreated {
		t.Fatalf("create protected workspace status=%d body=%s", protectedResponse.Code, protectedResponse.Body.String())
	}
	var protected struct {
		ID string `json:"id"`
	}
	decodeResponse(t, protectedResponse, &protected)
	enrollment, err := repository.EnrollRunner(context.Background(), owner.ID, domain.WorkspaceID(protected.ID), auth.HashToken("protected-runner-token"), "Protected Mac", "protected-mac", "darwin", "arm64", "test", []domain.AgentRegistration{{LocalID: "codex", Type: "codex", DisplayName: "Codex", Capabilities: []byte(`{"execution":true}`), Available: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.ImportAgent(context.Background(), domain.WorkspaceID(workspace.ID), enrollment.Agents[0].AgentID, owner.ID); err != nil {
		t.Fatal(err)
	}
	deletedSharedHome := requestJSON(t, handler, http.MethodDelete, "/api/v1/workspaces/"+protected.ID, nil, ownerCookie)
	if deletedSharedHome.Code != http.StatusNoContent {
		t.Fatalf("shared Runner workspace delete status=%d body=%s", deletedSharedHome.Code, deletedSharedHome.Body.String())
	}
	if err := database.Read(context.Background(), func(db *sql.DB) error {
		var deviceWorkspaceID string
		if err := db.QueryRow(`SELECT d.workspace_id FROM devices d JOIN runners r ON r.device_id = d.id WHERE r.id = ?`, enrollment.Runner.ID).Scan(&deviceWorkspaceID); err != nil {
			return err
		}
		if deviceWorkspaceID != workspace.ID {
			t.Fatalf("shared Runner device workspace=%q, want %q", deviceWorkspaceID, workspace.ID)
		}
		var survivingAgents int
		return db.QueryRow(`SELECT COUNT(*) FROM agents WHERE workspace_id = ? AND runner_id = ?`, workspace.ID, enrollment.Runner.ID).Scan(&survivingAgents)
	}); err != nil {
		t.Fatal(err)
	}
	sharedAgents, err := repository.ListAgents(context.Background(), domain.WorkspaceID(workspace.ID), owner.ID)
	if err != nil || len(sharedAgents) == 0 || sharedAgents[len(sharedAgents)-1].DeviceID != enrollment.Runner.DeviceID {
		t.Fatalf("shared Agent should survive workspace deletion: agents=%+v err=%v", sharedAgents, err)
	}
}

func requestJSON(t *testing.T, handler http.Handler, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &payload)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %q: %v", response.Body.String(), err)
	}
}
