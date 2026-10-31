package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/httpapi"
	store "agentcollab.local/agentcollab/internal/store/sqlite"
)

func runnerRequest(t *testing.T, handler http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(payload)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestRunnerTaskDeliveryFlow(t *testing.T) {
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	authService := auth.NewService(store.NewAuthRepository(database))
	if _, err := authService.CreateUser(context.Background(), "owner", "Owner", "password-1234", domain.UserAdmin); err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewRouter(authService, store.NewCollabRepository(database))
	cookie := loginCookie(t, handler, "owner", "password-1234")

	workspaceResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces", map[string]string{"name": "Local"}, cookie)
	var workspace struct {
		ID string `json:"id"`
	}
	decodeResponse(t, workspaceResponse, &workspace)
	roomResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces/"+workspace.ID+"/rooms", map[string]string{"name": "General"}, cookie)
	var room struct {
		ID string `json:"id"`
	}
	decodeResponse(t, roomResponse, &room)

	enrollResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/runner/enroll", map[string]any{
		"workspace_id": workspace.ID, "device_name": "Test Mac", "os": "darwin", "arch": "arm64", "version": "test",
		"agents": []map[string]any{{"local_id": "codex", "type": "codex", "display_name": "Codex", "available": true, "capabilities": map[string]any{
			"execution": true,
			"skills":    []map[string]any{{"name": "stock-analysis", "description": "Analyze local stock data"}},
			"resources": []map[string]any{{"name": "research-db", "description": "Local database", "access": "read_write"}},
		}}},
	}, cookie)
	if enrollResponse.Code != http.StatusCreated {
		t.Fatalf("enroll status=%d body=%s", enrollResponse.Code, enrollResponse.Body.String())
	}
	var enrollment struct {
		Token      string `json:"token"`
		Enrollment struct {
			Runner struct {
				ID string `json:"id"`
			} `json:"runner"`
			Agents []struct {
				AgentID string `json:"agent_id"`
			} `json:"agents"`
		} `json:"enrollment"`
	}
	decodeResponse(t, enrollResponse, &enrollment)
	ownership := requestJSON(t, handler, http.MethodGet, "/api/v1/runners/"+enrollment.Enrollment.Runner.ID+"/ownership", nil, cookie)
	if ownership.Code != http.StatusOK || !bytes.Contains(ownership.Body.Bytes(), []byte(`"owned_by_current_user":true`)) {
		t.Fatalf("runner ownership status=%d body=%s", ownership.Code, ownership.Body.String())
	}

	blockedTask := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+room.ID+"/tasks", map[string]any{"target_agent_id": enrollment.Enrollment.Agents[0].AgentID, "title": "Blocked", "prompt": "Return OK"}, cookie)
	if blockedTask.Code != http.StatusNotFound {
		t.Fatalf("task before adding Agent status=%d body=%s", blockedTask.Code, blockedTask.Body.String())
	}
	importAgent := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces/"+workspace.ID+"/agents/import", map[string]any{"agent_id": enrollment.Enrollment.Agents[0].AgentID}, cookie)
	if importAgent.Code != http.StatusNoContent {
		t.Fatalf("import Agent status=%d body=%s", importAgent.Code, importAgent.Body.String())
	}
	addAgent := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+room.ID+"/agents", map[string]any{"agent_id": enrollment.Enrollment.Agents[0].AgentID}, cookie)
	if addAgent.Code != http.StatusNoContent {
		t.Fatalf("add room Agent status=%d body=%s", addAgent.Code, addAgent.Body.String())
	}
	roomAgents := requestJSON(t, handler, http.MethodGet, "/api/v1/rooms/"+room.ID+"/agents", nil, cookie)
	if roomAgents.Code != http.StatusOK || !bytes.Contains(roomAgents.Body.Bytes(), []byte(`"display_name":"Codex"`)) || !bytes.Contains(roomAgents.Body.Bytes(), []byte(`"name":"stock-analysis"`)) || !bytes.Contains(roomAgents.Body.Bytes(), []byte(`"name":"research-db"`)) {
		t.Fatalf("room Agents status=%d body=%s", roomAgents.Code, roomAgents.Body.String())
	}
	if err := database.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE runners SET last_seen_at = '2000-01-01T00:00:00Z'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	staleRoomAgents := requestJSON(t, handler, http.MethodGet, "/api/v1/rooms/"+room.ID+"/agents", nil, cookie)
	if staleRoomAgents.Code != http.StatusOK || !bytes.Contains(staleRoomAgents.Body.Bytes(), []byte(`"status":"offline"`)) {
		t.Fatalf("stale Runner status=%d body=%s", staleRoomAgents.Code, staleRoomAgents.Body.String())
	}
	heartbeat := runnerRequest(t, handler, http.MethodPost, "/api/v1/runner/heartbeat", enrollment.Token, map[string]any{"version": "test", "agent_ids": []string{enrollment.Enrollment.Agents[0].AgentID}})
	if heartbeat.Code != http.StatusNoContent {
		t.Fatalf("heartbeat status=%d body=%s", heartbeat.Code, heartbeat.Body.String())
	}
	catalogHeartbeat := runnerRequest(t, handler, http.MethodPost, "/api/v1/runner/heartbeat", enrollment.Token, map[string]any{
		"version": "test",
		"agents": []map[string]any{
			{
				"local_id": "codex", "type": "codex", "display_name": "Codex",
				"available": true, "capabilities": map[string]any{
					"execution": true, "transport": "native", "catalog_tier": "native",
				},
			},
			{
				"local_id": "scan-codex-acp", "type": "codex-acp",
				"display_name": "Codex ACP Local", "available": true,
				"capabilities": map[string]any{
					"execution": true, "transport": "acp", "catalog_tier": "preset",
					"protocol_version": 1,
				},
			},
		},
	})
	if catalogHeartbeat.Code != http.StatusOK ||
		!bytes.Contains(catalogHeartbeat.Body.Bytes(), []byte(`"local_id":"scan-codex-acp"`)) {
		t.Fatalf("catalog heartbeat status=%d body=%s", catalogHeartbeat.Code, catalogHeartbeat.Body.String())
	}
	candidatesAfterCatalogSync := requestJSON(t, handler, http.MethodGet, "/api/v1/workspaces/"+workspace.ID+"/agent-candidates", nil, cookie)
	if candidatesAfterCatalogSync.Code != http.StatusOK ||
		!bytes.Contains(candidatesAfterCatalogSync.Body.Bytes(), []byte(`"type":"codex-acp"`)) ||
		!bytes.Contains(candidatesAfterCatalogSync.Body.Bytes(), []byte(`"transport":"acp"`)) ||
		!bytes.Contains(candidatesAfterCatalogSync.Body.Bytes(), []byte(`"protocol_version":1`)) {
		t.Fatalf("catalog candidates status=%d body=%s", candidatesAfterCatalogSync.Code, candidatesAfterCatalogSync.Body.String())
	}
	restoredRoomAgents := requestJSON(t, handler, http.MethodGet, "/api/v1/rooms/"+room.ID+"/agents", nil, cookie)
	if restoredRoomAgents.Code != http.StatusOK || !bytes.Contains(restoredRoomAgents.Body.Bytes(), []byte(`"status":"online"`)) {
		t.Fatalf("restored Runner status=%d body=%s", restoredRoomAgents.Code, restoredRoomAgents.Body.String())
	}
	brainstormWithoutCoordinator := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+room.ID+"/messages", map[string]any{"body": "/brainstorm 评估当前方案"}, cookie)
	if brainstormWithoutCoordinator.Code != http.StatusConflict || !bytes.Contains(brainstormWithoutCoordinator.Body.Bytes(), []byte(`"code":"coordinator_required"`)) {
		t.Fatalf("brainstorm without coordinator status=%d body=%s", brainstormWithoutCoordinator.Code, brainstormWithoutCoordinator.Body.String())
	}

	taskResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+room.ID+"/tasks", map[string]any{"target_agent_id": enrollment.Enrollment.Agents[0].AgentID, "title": "Smoke", "prompt": "Return OK"}, cookie)
	if taskResponse.Code != http.StatusCreated {
		t.Fatalf("task status=%d body=%s", taskResponse.Code, taskResponse.Body.String())
	}
	var created struct {
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
	}
	decodeResponse(t, taskResponse, &created)
	tasksResponse := requestJSON(t, handler, http.MethodGet, "/api/v1/rooms/"+room.ID+"/tasks", nil, cookie)
	if tasksResponse.Code != http.StatusOK || !bytes.Contains(tasksResponse.Body.Bytes(), []byte(`"title":"Smoke"`)) || !bytes.Contains(tasksResponse.Body.Bytes(), []byte(`"state":"queued"`)) {
		t.Fatalf("list tasks status=%d body=%s", tasksResponse.Code, tasksResponse.Body.String())
	}
	removeRunningRoomAgent := requestJSON(t, handler, http.MethodDelete, "/api/v1/rooms/"+room.ID+"/agents/"+enrollment.Enrollment.Agents[0].AgentID, nil, cookie)
	if removeRunningRoomAgent.Code != http.StatusConflict {
		t.Fatalf("remove room Agent with active task status=%d body=%s", removeRunningRoomAgent.Code, removeRunningRoomAgent.Body.String())
	}
	removeRunningWorkspaceAgent := requestJSON(t, handler, http.MethodDelete, "/api/v1/workspaces/"+workspace.ID+"/agents/"+enrollment.Enrollment.Agents[0].AgentID, nil, cookie)
	if removeRunningWorkspaceAgent.Code != http.StatusConflict {
		t.Fatalf("remove workspace Agent with active task status=%d body=%s", removeRunningWorkspaceAgent.Code, removeRunningWorkspaceAgent.Body.String())
	}

	nextRequest := httptest.NewRequest(http.MethodGet, "/api/v1/runner/tasks/next", nil)
	nextRequest.Header.Set("Authorization", "Bearer "+enrollment.Token)
	nextResponse := httptest.NewRecorder()
	handler.ServeHTTP(nextResponse, nextRequest)
	if nextResponse.Code != http.StatusOK {
		t.Fatalf("next status=%d body=%s", nextResponse.Code, nextResponse.Body.String())
	}

	startRequest := httptest.NewRequest(http.MethodPost, "/api/v1/runner/tasks/"+created.Task.ID+"/start", nil)
	startRequest.Header.Set("Authorization", "Bearer "+enrollment.Token)
	startResponse := httptest.NewRecorder()
	handler.ServeHTTP(startResponse, startRequest)
	if startResponse.Code != http.StatusNoContent {
		t.Fatalf("start status=%d body=%s", startResponse.Code, startResponse.Body.String())
	}

	completeBody, _ := json.Marshal(map[string]any{"success": true, "output": "OK", "exit_code": 0, "receipt": map[string]any{
		"native_agent_type": "codex", "executable": "/usr/bin/codex", "version": "test", "pid": 42,
		"skill_count": 1, "resource_count": 2, "matched_skills": []string{"stock-analysis"}, "matched_resources": []string{"research-db"},
	}})
	completeRequest := httptest.NewRequest(http.MethodPost, "/api/v1/runner/tasks/"+created.Task.ID+"/complete", bytes.NewReader(completeBody))
	completeRequest.Header.Set("Authorization", "Bearer "+enrollment.Token)
	completeRequest.Header.Set("Content-Type", "application/json")
	completeResponse := httptest.NewRecorder()
	handler.ServeHTTP(completeResponse, completeRequest)
	if completeResponse.Code != http.StatusOK {
		t.Fatalf("complete status=%d body=%s", completeResponse.Code, completeResponse.Body.String())
	}
	messagesAfterCompletion := requestJSON(t, handler, http.MethodGet, "/api/v1/rooms/"+room.ID+"/messages?limit=100", nil, cookie)
	if messagesAfterCompletion.Code != http.StatusOK || !bytes.Contains(messagesAfterCompletion.Body.Bytes(), []byte(`执行设备：Test Mac`)) || !bytes.Contains(messagesAfterCompletion.Body.Bytes(), []byte(`本机能力上下文：1 Skills · 2 Resources`)) || !bytes.Contains(messagesAfterCompletion.Body.Bytes(), []byte(`任务/交付提及 Skill：stock-analysis`)) {
		t.Fatalf("completion receipt status=%d body=%s", messagesAfterCompletion.Code, messagesAfterCompletion.Body.String())
	}

	removeRoomAgent := requestJSON(t, handler, http.MethodDelete, "/api/v1/rooms/"+room.ID+"/agents/"+enrollment.Enrollment.Agents[0].AgentID, nil, cookie)
	if removeRoomAgent.Code != http.StatusNoContent {
		t.Fatalf("remove room Agent status=%d body=%s", removeRoomAgent.Code, removeRoomAgent.Body.String())
	}
	roomAgentsAfterRemoval := requestJSON(t, handler, http.MethodGet, "/api/v1/rooms/"+room.ID+"/agents", nil, cookie)
	if roomAgentsAfterRemoval.Code != http.StatusOK || bytes.Contains(roomAgentsAfterRemoval.Body.Bytes(), []byte(`"display_name":"Codex"`)) {
		t.Fatalf("room Agents after removal status=%d body=%s", roomAgentsAfterRemoval.Code, roomAgentsAfterRemoval.Body.String())
	}
	if response := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+room.ID+"/agents", map[string]any{"agent_id": enrollment.Enrollment.Agents[0].AgentID}, cookie); response.Code != http.StatusNoContent {
		t.Fatalf("re-add room Agent status=%d body=%s", response.Code, response.Body.String())
	}
	removeWorkspaceAgent := requestJSON(t, handler, http.MethodDelete, "/api/v1/workspaces/"+workspace.ID+"/agents/"+enrollment.Enrollment.Agents[0].AgentID, nil, cookie)
	if removeWorkspaceAgent.Code != http.StatusNoContent {
		t.Fatalf("remove workspace Agent status=%d body=%s", removeWorkspaceAgent.Code, removeWorkspaceAgent.Body.String())
	}
	workspaceAgentsAfterRemoval := requestJSON(t, handler, http.MethodGet, "/api/v1/workspaces/"+workspace.ID+"/agents", nil, cookie)
	if workspaceAgentsAfterRemoval.Code != http.StatusOK || bytes.Contains(workspaceAgentsAfterRemoval.Body.Bytes(), []byte(`"display_name":"Codex"`)) {
		t.Fatalf("workspace Agents after removal status=%d body=%s", workspaceAgentsAfterRemoval.Code, workspaceAgentsAfterRemoval.Body.String())
	}
	candidatesAfterRemoval := requestJSON(t, handler, http.MethodGet, "/api/v1/workspaces/"+workspace.ID+"/agent-candidates", nil, cookie)
	if candidatesAfterRemoval.Code != http.StatusOK || bytes.Contains(candidatesAfterRemoval.Body.Bytes(), []byte(`"display_name":"Codex"`)) {
		t.Fatalf("Agent candidates after removal status=%d body=%s", candidatesAfterRemoval.Code, candidatesAfterRemoval.Body.String())
	}
}

func TestStructuredMentionsAgentHandoffAndNativeSessionResume(t *testing.T) {
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	authService := auth.NewService(store.NewAuthRepository(database))
	if _, err := authService.CreateUser(context.Background(), "owner", "Owner", "password-1234", domain.UserAdmin); err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewRouter(authService, store.NewCollabRepository(database))
	cookie := loginCookie(t, handler, "owner", "password-1234")

	workspaceResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces", map[string]string{"name": "Collab"}, cookie)
	var workspace struct {
		ID string `json:"id"`
	}
	decodeResponse(t, workspaceResponse, &workspace)
	roomResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces/"+workspace.ID+"/rooms", map[string]string{"name": "Brainstorm"}, cookie)
	var room struct {
		ID string `json:"id"`
	}
	decodeResponse(t, roomResponse, &room)

	enrollResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/runner/enroll", map[string]any{
		"workspace_id": workspace.ID, "device_name": "Test Mac", "os": "darwin", "arch": "arm64", "version": "test",
		"agents": []map[string]any{
			{"local_id": "first", "type": "codex", "display_name": "First Agent", "available": true, "capabilities": map[string]any{"execution": true}},
			{"local_id": "second", "type": "hermes", "display_name": "Second Agent", "available": true, "capabilities": map[string]any{"execution": true}},
		},
	}, cookie)
	var enrollment struct {
		Token      string `json:"token"`
		Enrollment struct {
			Agents []struct {
				AgentID string `json:"agent_id"`
			} `json:"agents"`
		} `json:"enrollment"`
	}
	decodeResponse(t, enrollResponse, &enrollment)
	firstID := enrollment.Enrollment.Agents[0].AgentID
	secondID := enrollment.Enrollment.Agents[1].AgentID
	for _, agentID := range []string{firstID, secondID} {
		if response := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces/"+workspace.ID+"/agents/import", map[string]any{"agent_id": agentID}, cookie); response.Code != http.StatusNoContent {
			t.Fatalf("import Agent status=%d body=%s", response.Code, response.Body.String())
		}
		if response := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+room.ID+"/agents", map[string]any{"agent_id": agentID}, cookie); response.Code != http.StatusNoContent {
			t.Fatalf("add Agent status=%d body=%s", response.Code, response.Body.String())
		}
	}
	settingsResponse := requestJSON(t, handler, http.MethodPatch, "/api/v1/rooms/"+room.ID+"/agents/"+firstID+"/settings", map[string]any{
		"response_mode": "human_all", "reply_delay_seconds": 0, "role_prompt": "负责提出初稿", "max_auto_turns": 4,
	}, cookie)
	if settingsResponse.Code != http.StatusNoContent {
		t.Fatalf("settings status=%d body=%s", settingsResponse.Code, settingsResponse.Body.String())
	}
	roomAgents := requestJSON(t, handler, http.MethodGet, "/api/v1/rooms/"+room.ID+"/agents", nil, cookie)
	if !bytes.Contains(roomAgents.Body.Bytes(), []byte(`"response_mode":"human_all"`)) || !bytes.Contains(roomAgents.Body.Bytes(), []byte(`"role_prompt":"负责提出初稿"`)) {
		t.Fatalf("room Agent settings body=%s", roomAgents.Body.String())
	}
	requestJSON(t, handler, http.MethodPatch, "/api/v1/rooms/"+room.ID+"/agents/"+firstID+"/settings", map[string]any{
		"response_mode": "mention", "reply_delay_seconds": 0, "role_prompt": "负责提出初稿", "max_auto_turns": 4,
	}, cookie)

	original := "@First Agent 请先分析，然后交给另一位"
	messageResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+room.ID+"/messages", map[string]any{
		"body": original, "mention_agent_ids": []string{firstID},
	}, cookie)
	if messageResponse.Code != http.StatusCreated || !bytes.Contains(messageResponse.Body.Bytes(), []byte(original)) || !bytes.Contains(messageResponse.Body.Bytes(), []byte(firstID)) {
		t.Fatalf("structured message status=%d body=%s", messageResponse.Code, messageResponse.Body.String())
	}
	var rootMessage struct {
		ID string `json:"id"`
	}
	decodeResponse(t, messageResponse, &rootMessage)
	if response := requestJSON(t, handler, http.MethodPatch, "/api/v1/rooms/"+room.ID+"/agents/"+firstID+"/settings", map[string]any{
		"response_mode": "human_all", "reply_delay_seconds": 0, "role_prompt": "负责提出初稿", "max_auto_turns": 4,
	}, cookie); response.Code != http.StatusNoContent {
		t.Fatalf("set human-all status=%d body=%s", response.Code, response.Body.String())
	}
	threadReply := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+room.ID+"/messages", map[string]any{
		"body": "真人补充：请重点核对风险。", "reply_to_message_id": rootMessage.ID, "root_message_id": rootMessage.ID,
	}, cookie)
	if threadReply.Code != http.StatusCreated || !bytes.Contains(threadReply.Body.Bytes(), []byte(`"reply_to_message_id":"`+rootMessage.ID+`"`)) || !bytes.Contains(threadReply.Body.Bytes(), []byte(`"root_message_id":"`+rootMessage.ID+`"`)) {
		t.Fatalf("thread reply status=%d body=%s", threadReply.Code, threadReply.Body.String())
	}
	// A human discussion reply does not silently wake human_all Agents. The
	// user must explicitly @ an Agent from within the topic to create work.
	tasksAfterReply := requestJSON(t, handler, http.MethodGet, "/api/v1/rooms/"+room.ID+"/tasks", nil, cookie)
	var listedTasks struct {
		Tasks []domain.Task `json:"tasks"`
	}
	decodeResponse(t, tasksAfterReply, &listedTasks)
	if len(listedTasks.Tasks) != 1 {
		t.Fatalf("plain thread reply created an unexpected task: %+v", listedTasks.Tasks)
	}

	firstClaim := runnerRequest(t, handler, http.MethodGet, "/api/v1/runner/tasks/next", enrollment.Token, nil)
	var firstTask domain.ClaimedTask
	decodeResponse(t, firstClaim, &firstTask)
	if firstTask.TargetAgentID != domain.AgentID(firstID) || firstTask.CollaborationDepth != 0 || firstTask.RootMessageID == nil {
		t.Fatalf("first task = %+v", firstTask)
	}
	if response := runnerRequest(t, handler, http.MethodPost, "/api/v1/runner/tasks/"+string(firstTask.ID)+"/start", enrollment.Token, map[string]any{}); response.Code != http.StatusNoContent {
		t.Fatalf("start first status=%d body=%s", response.Code, response.Body.String())
	}
	handoffOutput := "初步分析完成。\n\n@Second Agent 请复核关键结论"
	completeFirst := runnerRequest(t, handler, http.MethodPost, "/api/v1/runner/tasks/"+string(firstTask.ID)+"/complete", enrollment.Token, map[string]any{
		"success": true, "output": handoffOutput, "exit_code": 0, "receipt": map[string]any{"native_session_id": "native-first"},
	})
	if completeFirst.Code != http.StatusOK || !bytes.Contains(completeFirst.Body.Bytes(), []byte(secondID)) {
		t.Fatalf("complete first status=%d body=%s", completeFirst.Code, completeFirst.Body.String())
	}

	secondClaim := runnerRequest(t, handler, http.MethodGet, "/api/v1/runner/tasks/next", enrollment.Token, nil)
	var secondTask domain.ClaimedTask
	decodeResponse(t, secondClaim, &secondTask)
	if secondTask.TargetAgentID != domain.AgentID(secondID) || secondTask.CreatorKind != domain.TaskCreatorAgent || secondTask.ParentTaskID == nil || secondTask.CollaborationDepth != 1 {
		t.Fatalf("handoff task = %+v", secondTask)
	}
	if response := runnerRequest(t, handler, http.MethodPost, "/api/v1/runner/tasks/"+string(secondTask.ID)+"/start", enrollment.Token, map[string]any{}); response.Code != http.StatusNoContent {
		t.Fatalf("start second status=%d body=%s", response.Code, response.Body.String())
	}
	if response := runnerRequest(t, handler, http.MethodPost, "/api/v1/runner/tasks/"+string(secondTask.ID)+"/complete", enrollment.Token, map[string]any{"success": true, "output": "复核完成", "exit_code": 0}); response.Code != http.StatusOK {
		t.Fatalf("complete second status=%d body=%s", response.Code, response.Body.String())
	}
	callbackClaim := runnerRequest(t, handler, http.MethodGet, "/api/v1/runner/tasks/next", enrollment.Token, nil)
	var callbackTask domain.ClaimedTask
	decodeResponse(t, callbackClaim, &callbackTask)
	if callbackTask.TargetAgentID != domain.AgentID(firstID) || callbackTask.CreatorKind != domain.TaskCreatorAgent || callbackTask.CollaborationDepth != 2 || callbackTask.NativeSessionID != "native-first" {
		t.Fatalf("automatic callback task = %+v", callbackTask)
	}
	if response := runnerRequest(t, handler, http.MethodPost, "/api/v1/runner/tasks/"+string(callbackTask.ID)+"/start", enrollment.Token, map[string]any{}); response.Code != http.StatusNoContent {
		t.Fatalf("start callback status=%d body=%s", response.Code, response.Body.String())
	}
	if response := runnerRequest(t, handler, http.MethodPost, "/api/v1/runner/tasks/"+string(callbackTask.ID)+"/complete", enrollment.Token, map[string]any{"success": true, "output": "最终汇总", "exit_code": 0, "receipt": map[string]any{"native_session_id": "native-first"}}); response.Code != http.StatusOK {
		t.Fatalf("complete callback status=%d body=%s", response.Code, response.Body.String())
	}

	requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+room.ID+"/messages", map[string]any{"body": "@First Agent 继续", "mention_agent_ids": []string{firstID}}, cookie)
	resumeClaim := runnerRequest(t, handler, http.MethodGet, "/api/v1/runner/tasks/next", enrollment.Token, nil)
	var resumeTask domain.ClaimedTask
	decodeResponse(t, resumeClaim, &resumeTask)
	if resumeTask.NativeSessionID != "native-first" {
		t.Fatalf("native session = %q, want native-first", resumeTask.NativeSessionID)
	}
}

func TestCoordinatorFansOutToEveryoneAndWaitsForAllExperts(t *testing.T) {
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	authService := auth.NewService(store.NewAuthRepository(database))
	if _, err := authService.CreateUser(context.Background(), "owner", "Owner", "password-1234", domain.UserAdmin); err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewRouter(authService, store.NewCollabRepository(database))
	cookie := loginCookie(t, handler, "owner", "password-1234")

	workspaceResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces", map[string]string{"name": "Collab"}, cookie)
	var workspace struct {
		ID string `json:"id"`
	}
	decodeResponse(t, workspaceResponse, &workspace)
	roomResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces/"+workspace.ID+"/rooms", map[string]string{"name": "Review"}, cookie)
	var room struct {
		ID string `json:"id"`
	}
	decodeResponse(t, roomResponse, &room)

	enrollResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/runner/enroll", map[string]any{
		"workspace_id": workspace.ID, "device_name": "Test Mac", "os": "darwin", "arch": "arm64", "version": "test",
		"agents": []map[string]any{
			{"local_id": "lead", "type": "hermes", "display_name": "Lead", "available": true, "capabilities": map[string]any{"execution": true}},
			{"local_id": "reviewer-a", "type": "codex", "display_name": "Reviewer A", "available": true, "capabilities": map[string]any{"execution": true}},
			{"local_id": "reviewer-b", "type": "workbuddy", "display_name": "Reviewer B", "available": true, "capabilities": map[string]any{"execution": true}},
		},
	}, cookie)
	var enrollment struct {
		Token      string `json:"token"`
		Enrollment struct {
			Agents []struct {
				AgentID string `json:"agent_id"`
			} `json:"agents"`
		} `json:"enrollment"`
	}
	decodeResponse(t, enrollResponse, &enrollment)
	leadID := enrollment.Enrollment.Agents[0].AgentID
	reviewerAID := enrollment.Enrollment.Agents[1].AgentID
	reviewerBID := enrollment.Enrollment.Agents[2].AgentID
	for _, agentID := range []string{leadID, reviewerAID, reviewerBID} {
		if response := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces/"+workspace.ID+"/agents/import", map[string]any{"agent_id": agentID}, cookie); response.Code != http.StatusNoContent {
			t.Fatalf("import Agent status=%d body=%s", response.Code, response.Body.String())
		}
		if response := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+room.ID+"/agents", map[string]any{"agent_id": agentID}, cookie); response.Code != http.StatusNoContent {
			t.Fatalf("add Agent status=%d body=%s", response.Code, response.Body.String())
		}
	}
	if response := requestJSON(t, handler, http.MethodPatch, "/api/v1/rooms/"+room.ID+"/agents/"+leadID+"/settings", map[string]any{
		"response_mode": "coordinator", "reply_delay_seconds": 0, "role_prompt": "负责统筹", "max_auto_turns": 4,
	}, cookie); response.Code != http.StatusNoContent {
		t.Fatalf("coordinator settings status=%d body=%s", response.Code, response.Body.String())
	}
	if response := requestJSON(t, handler, http.MethodPatch, "/api/v1/rooms/"+room.ID+"/agents/"+reviewerAID+"/settings", map[string]any{
		"response_mode": "coordinator", "reply_delay_seconds": 0, "role_prompt": "临时协调", "max_auto_turns": 4,
	}, cookie); response.Code != http.StatusNoContent {
		t.Fatalf("second coordinator settings status=%d body=%s", response.Code, response.Body.String())
	}
	if response := requestJSON(t, handler, http.MethodPatch, "/api/v1/rooms/"+room.ID+"/agents/"+leadID+"/settings", map[string]any{
		"response_mode": "coordinator", "reply_delay_seconds": 0, "role_prompt": "负责统筹", "max_auto_turns": 4,
	}, cookie); response.Code != http.StatusNoContent {
		t.Fatalf("restore lead coordinator status=%d body=%s", response.Code, response.Body.String())
	}
	coordinatorAgents := requestJSON(t, handler, http.MethodGet, "/api/v1/rooms/"+room.ID+"/agents", nil, cookie)
	if got := bytes.Count(coordinatorAgents.Body.Bytes(), []byte(`"response_mode":"coordinator"`)); got != 1 {
		t.Fatalf("coordinator count=%d body=%s", got, coordinatorAgents.Body.String())
	}

	messageResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+room.ID+"/messages", map[string]any{
		"body": "/brainstorm 评估股票技能的实盘效果与问题",
	}, cookie)
	if messageResponse.Code != http.StatusCreated {
		t.Fatalf("message status=%d body=%s", messageResponse.Code, messageResponse.Body.String())
	}
	leadClaim := runnerRequest(t, handler, http.MethodGet, "/api/v1/runner/tasks/next", enrollment.Token, nil)
	var leadTask domain.ClaimedTask
	decodeResponse(t, leadClaim, &leadTask)
	if leadTask.TargetAgentID != domain.AgentID(leadID) || !strings.Contains(leadTask.Prompt, "群聊级专题讨论") {
		t.Fatalf("lead task = %+v", leadTask)
	}
	if prematureExpert := runnerRequest(t, handler, http.MethodGet, "/api/v1/runner/tasks/next", enrollment.Token, nil); prematureExpert.Code != http.StatusNoContent {
		t.Fatalf("brainstorm experts started before coordinator opening: status=%d body=%s", prematureExpert.Code, prematureExpert.Body.String())
	}
	runnerRequest(t, handler, http.MethodPost, "/api/v1/runner/tasks/"+string(leadTask.ID)+"/start", enrollment.Token, map[string]any{})
	leadOutput := "我先开场。\n\n@Reviewer A 请从工程角度评估"
	if response := runnerRequest(t, handler, http.MethodPost, "/api/v1/runner/tasks/"+string(leadTask.ID)+"/complete", enrollment.Token, map[string]any{"success": true, "output": leadOutput, "exit_code": 0}); response.Code != http.StatusOK {
		t.Fatalf("complete lead status=%d body=%s", response.Code, response.Body.String())
	}

	claimedExperts := make(map[domain.AgentID]domain.ClaimedTask)
	for range 2 {
		response := runnerRequest(t, handler, http.MethodGet, "/api/v1/runner/tasks/next", enrollment.Token, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("claim expert status=%d body=%s", response.Code, response.Body.String())
		}
		var task domain.ClaimedTask
		decodeResponse(t, response, &task)
		claimedExperts[task.TargetAgentID] = task
	}
	if _, ok := claimedExperts[domain.AgentID(reviewerAID)]; !ok {
		t.Fatalf("Reviewer A did not receive a task: %+v", claimedExperts)
	}
	if reviewerB, ok := claimedExperts[domain.AgentID(reviewerBID)]; !ok {
		t.Fatalf("Reviewer B did not receive the implicit all-member task: %+v", claimedExperts)
	} else if !strings.Contains(reviewerB.Prompt, "用户原始要求") || !strings.Contains(reviewerB.Prompt, "明确结论") {
		t.Fatalf("Reviewer B prompt does not contain the original group assignment: %s", reviewerB.Prompt)
	}
	for _, task := range claimedExperts {
		if response := runnerRequest(t, handler, http.MethodPost, "/api/v1/runner/tasks/"+string(task.ID)+"/start", enrollment.Token, map[string]any{}); response.Code != http.StatusNoContent {
			t.Fatalf("start expert status=%d body=%s", response.Code, response.Body.String())
		}
	}

	first := claimedExperts[domain.AgentID(reviewerAID)]
	if response := runnerRequest(t, handler, http.MethodPost, "/api/v1/runner/tasks/"+string(first.ID)+"/complete", enrollment.Token, map[string]any{"success": true, "output": "Codex 结论：工程链路有超时风险。", "exit_code": 0}); response.Code != http.StatusOK {
		t.Fatalf("complete first expert status=%d body=%s", response.Code, response.Body.String())
	}
	if early := runnerRequest(t, handler, http.MethodGet, "/api/v1/runner/tasks/next", enrollment.Token, nil); early.Code != http.StatusNoContent {
		t.Fatalf("coordinator callback became claimable before all experts finished: status=%d body=%s", early.Code, early.Body.String())
	}

	second := claimedExperts[domain.AgentID(reviewerBID)]
	if response := runnerRequest(t, handler, http.MethodPost, "/api/v1/runner/tasks/"+string(second.ID)+"/complete", enrollment.Token, map[string]any{"success": true, "output": "WorkBuddy 结论：需要补齐实盘成交闭环。", "exit_code": 0}); response.Code != http.StatusOK {
		t.Fatalf("complete second expert status=%d body=%s", response.Code, response.Body.String())
	}
	callbackResponse := runnerRequest(t, handler, http.MethodGet, "/api/v1/runner/tasks/next", enrollment.Token, nil)
	if callbackResponse.Code != http.StatusOK {
		t.Fatalf("callback claim status=%d body=%s", callbackResponse.Code, callbackResponse.Body.String())
	}
	var callback domain.ClaimedTask
	decodeResponse(t, callbackResponse, &callback)
	if callback.TargetAgentID != domain.AgentID(leadID) || !strings.HasPrefix(callback.Title, "Agent 回传：") {
		t.Fatalf("callback task = %+v", callback)
	}
	for _, expected := range []string{"Codex 结论", "WorkBuddy 结论", "所有直接专家任务已经结束"} {
		if !strings.Contains(callback.Prompt, expected) {
			t.Fatalf("callback prompt missing %q: %s", expected, callback.Prompt)
		}
	}
}
