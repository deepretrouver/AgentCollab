package httpapi_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/httpapi"
	"agentcollab.local/agentcollab/internal/storage"
	store "agentcollab.local/agentcollab/internal/store/sqlite"
)

func TestKnowledgeAssetsUseProjectDirectoriesAndReachRoomAgent(t *testing.T) {
	ctx := context.Background()
	storageRoot := t.TempDir()
	database, err := store.Open(ctx, filepath.Join(storageRoot, "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	authService := auth.NewService(store.NewAuthRepository(database))
	owner, err := authService.CreateUser(ctx, "owner", "Owner", "password-1234", domain.UserAdmin)
	if err != nil {
		t.Fatal(err)
	}
	repository := store.NewCollabRepository(database)
	handler := httpapi.NewRouterWithConfig(authService, repository, config.Hub{StorageRoot: storageRoot})
	cookie := loginCookie(t, handler, "owner", "password-1234")

	workspaceResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces", map[string]any{"name": "Research"}, cookie)
	var workspace domain.Workspace
	decodeResponse(t, workspaceResponse, &workspace)
	roomResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces/"+string(workspace.ID)+"/rooms", map[string]any{"name": "Committee"}, cookie)
	var room domain.Room
	decodeResponse(t, roomResponse, &room)

	roomDir, err := storage.RoomDirectory(string(workspace.ID), string(room.ID))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"knowledge", "uploads", "deliverables"} {
		if info, err := os.Stat(filepath.Join(storageRoot, roomDir, name)); err != nil || !info.IsDir() {
			t.Fatalf("room directory %s missing: %v", name, err)
		}
	}

	workspaceBody := []byte("workspace policy")
	workspaceUpload := uploadTestAttachment(t, handler, "/api/v1/workspaces/"+string(workspace.ID)+"/knowledge", "规范.md", workspaceBody, cookie)
	if workspaceUpload.Code != http.StatusCreated {
		t.Fatalf("workspace upload status=%d body=%s", workspaceUpload.Code, workspaceUpload.Body.String())
	}
	var workspaceAsset domain.KnowledgeAsset
	decodeResponse(t, workspaceUpload, &workspaceAsset)
	roomBody := []byte("room reference")
	roomUpload := uploadTestAttachment(t, handler, "/api/v1/rooms/"+string(room.ID)+"/knowledge", "样例.txt", roomBody, cookie)
	if roomUpload.Code != http.StatusCreated {
		t.Fatalf("room upload status=%d body=%s", roomUpload.Code, roomUpload.Body.String())
	}
	var roomAsset domain.KnowledgeAsset
	decodeResponse(t, roomUpload, &roomAsset)
	if workspaceAsset.Scope != domain.KnowledgeWorkspace || roomAsset.Scope != domain.KnowledgeRoom {
		t.Fatalf("knowledge scopes workspace=%q room=%q", workspaceAsset.Scope, roomAsset.Scope)
	}
	workspaceDirectory, _ := storage.WorkspaceDirectory(string(workspace.ID))
	workspaceFiles, workspaceGlobErr := filepath.Glob(filepath.Join(storageRoot, workspaceDirectory, "knowledge", "*", "规范.md"))
	roomFiles, roomGlobErr := filepath.Glob(filepath.Join(storageRoot, roomDir, "knowledge", "*", "样例.txt"))
	if workspaceGlobErr != nil || roomGlobErr != nil || len(workspaceFiles) != 1 || len(roomFiles) != 1 {
		t.Fatalf("knowledge files workspace=%v room=%v errors=%v/%v", workspaceFiles, roomFiles, workspaceGlobErr, roomGlobErr)
	}

	workspaceList := requestJSON(t, handler, http.MethodGet, "/api/v1/workspaces/"+string(workspace.ID)+"/knowledge", nil, cookie)
	if workspaceList.Code != http.StatusOK || !bytes.Contains(workspaceList.Body.Bytes(), []byte(`"directory":"workspaces`)) {
		t.Fatalf("workspace list status=%d body=%s", workspaceList.Code, workspaceList.Body.String())
	}
	roomList := requestJSON(t, handler, http.MethodGet, "/api/v1/rooms/"+string(room.ID)+"/knowledge", nil, cookie)
	if roomList.Code != http.StatusOK || !bytes.Contains(roomList.Body.Bytes(), []byte(`"filename":"样例.txt"`)) {
		t.Fatalf("room list status=%d body=%s", roomList.Code, roomList.Body.String())
	}

	const runnerToken = "runner-secret"
	enrollment, err := repository.EnrollRunner(ctx, owner.ID, workspace.ID, auth.HashToken(runnerToken), "Review-PC", "review-pc", "windows", "amd64", "test", []domain.AgentRegistration{{LocalID: "codex", Type: "codex", DisplayName: "Codex", Capabilities: []byte(`{"execution":true}`), Available: true}})
	if err != nil {
		t.Fatal(err)
	}
	agentID := enrollment.Agents[0].AgentID
	if err := repository.ImportAgent(ctx, workspace.ID, agentID, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddRoomAgent(ctx, room.ID, agentID, owner.ID); err != nil {
		t.Fatal(err)
	}
	messageResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+string(room.ID)+"/messages", map[string]any{"body": "@Codex 根据知识库检查", "mention_agent_ids": []string{string(agentID)}}, cookie)
	if messageResponse.Code != http.StatusCreated {
		t.Fatalf("message status=%d body=%s", messageResponse.Code, messageResponse.Body.String())
	}
	nextRequest := httptest.NewRequest(http.MethodGet, "/api/v1/runner/tasks/next", nil)
	nextRequest.Header.Set("Authorization", "Bearer "+runnerToken)
	nextResponse := httptest.NewRecorder()
	handler.ServeHTTP(nextResponse, nextRequest)
	if nextResponse.Code != http.StatusOK {
		t.Fatalf("next task status=%d body=%s", nextResponse.Code, nextResponse.Body.String())
	}
	var task domain.ClaimedTask
	decodeResponse(t, nextResponse, &task)
	if len(task.KnowledgeAssets) != 2 {
		t.Fatalf("claimed knowledge=%#v", task.KnowledgeAssets)
	}
	for _, asset := range task.KnowledgeAssets {
		downloadRequest := httptest.NewRequest(http.MethodGet, "/api/v1/runner/tasks/"+string(task.ID)+"/knowledge/"+string(asset.ID), nil)
		downloadRequest.Header.Set("Authorization", "Bearer "+runnerToken)
		downloadResponse := httptest.NewRecorder()
		handler.ServeHTTP(downloadResponse, downloadRequest)
		if downloadResponse.Code != http.StatusOK {
			t.Fatalf("runner knowledge download %s status=%d body=%s", asset.ID, downloadResponse.Code, downloadResponse.Body.String())
		}
	}
}
