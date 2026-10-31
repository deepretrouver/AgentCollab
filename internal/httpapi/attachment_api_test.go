package httpapi_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/httpapi"
	"agentcollab.local/agentcollab/internal/storage"
	store "agentcollab.local/agentcollab/internal/store/sqlite"
)

func TestMessageAttachmentReachesOnlyTheTargetRunnerTask(t *testing.T) {
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

	const runnerToken = "runner-secret"
	enrollment, err := repository.EnrollRunner(ctx, owner.ID, workspace.ID, auth.HashToken(runnerToken), "Review-PC", "review-pc-installation", "windows", "amd64", "test", []domain.AgentRegistration{{LocalID: "hermes", Type: "hermes", DisplayName: "Hermes Chengong", Capabilities: []byte(`{"execution":true}`), Available: true}})
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

	report := []byte("confidential investment committee report")
	uploadResponse := uploadTestAttachment(t, handler, "/api/v1/rooms/"+string(room.ID)+"/attachments", "授信报告.pdf", report, cookie)
	if uploadResponse.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", uploadResponse.Code, uploadResponse.Body.String())
	}
	var attachment domain.Attachment
	decodeResponse(t, uploadResponse, &attachment)
	roomDirectory, err := storage.RoomDirectory(string(workspace.ID), string(room.ID))
	if err != nil {
		t.Fatal(err)
	}
	storedAttachments, err := filepath.Glob(filepath.Join(storageRoot, roomDirectory, "uploads", "*", "授信报告.pdf"))
	if err != nil || len(storedAttachments) != 1 {
		t.Fatalf("chat upload path matches=%v err=%v", storedAttachments, err)
	}
	messageResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+string(room.ID)+"/messages", map[string]any{
		"body": "@Hermes Chengong 请调用投委会审查 Skill 审查附件", "mention_agent_ids": []string{string(agentID)}, "attachment_ids": []string{string(attachment.ID)},
	}, cookie)
	if messageResponse.Code != http.StatusCreated {
		t.Fatalf("message status=%d body=%s", messageResponse.Code, messageResponse.Body.String())
	}
	var message domain.Message
	decodeResponse(t, messageResponse, &message)
	if len(message.Attachments) != 1 || message.Attachments[0].Filename != "授信报告.pdf" || message.Attachments[0].DownloadURL == "" {
		t.Fatalf("message attachments=%#v", message.Attachments)
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
	if len(task.Attachments) != 1 || task.Attachments[0].SHA256 != attachment.SHA256 {
		t.Fatalf("claimed attachments=%#v", task.Attachments)
	}

	downloadRequest := httptest.NewRequest(http.MethodGet, "/api/v1/runner/tasks/"+string(task.ID)+"/attachments/"+string(attachment.ID), nil)
	downloadRequest.Header.Set("Authorization", "Bearer "+runnerToken)
	downloadResponse := httptest.NewRecorder()
	handler.ServeHTTP(downloadResponse, downloadRequest)
	if downloadResponse.Code != http.StatusOK || !bytes.Equal(downloadResponse.Body.Bytes(), report) {
		t.Fatalf("runner download status=%d body=%q", downloadResponse.Code, downloadResponse.Body.Bytes())
	}

	wrongTaskRequest := httptest.NewRequest(http.MethodGet, "/api/v1/runner/tasks/task-wrong/attachments/"+string(attachment.ID), nil)
	wrongTaskRequest.Header.Set("Authorization", "Bearer "+runnerToken)
	wrongTaskResponse := httptest.NewRecorder()
	handler.ServeHTTP(wrongTaskResponse, wrongTaskRequest)
	if wrongTaskResponse.Code != http.StatusNotFound {
		t.Fatalf("wrong task download status=%d body=%s", wrongTaskResponse.Code, wrongTaskResponse.Body.String())
	}

	reuseResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+string(room.ID)+"/messages", map[string]any{"body": "reuse", "attachment_ids": []string{string(attachment.ID)}}, cookie)
	if reuseResponse.Code != http.StatusBadRequest {
		t.Fatalf("reused attachment status=%d body=%s", reuseResponse.Code, reuseResponse.Body.String())
	}
}

func uploadTestAttachment(t *testing.T, handler http.Handler, path, filename string, body []byte, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &payload)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
