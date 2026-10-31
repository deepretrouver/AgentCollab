package httpapi_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/httpapi"
	"agentcollab.local/agentcollab/internal/storage"
	store "agentcollab.local/agentcollab/internal/store/sqlite"
)

func TestDirectoryUploadDrivesStepBranchReviewAndMerge(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	ctx := context.Background()
	root := t.TempDir()
	database, err := store.Open(ctx, filepath.Join(root, "agentcollab.db"))
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
	handler := httpapi.NewRouterWithConfig(authService, repository, config.Hub{StorageRoot: root})
	cookie := loginCookie(t, handler, "owner", "password-1234")
	workspaceResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces", map[string]any{"name": "Project"}, cookie)
	var workspace domain.Workspace
	decodeResponse(t, workspaceResponse, &workspace)
	roomResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces/"+string(workspace.ID)+"/rooms", map[string]any{"name": "Development"}, cookie)
	var room domain.Room
	decodeResponse(t, roomResponse, &room)
	stepResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces/"+string(workspace.ID)+"/project-steps", map[string]any{"title": "Module", "weight": 100}, cookie)
	var step domain.ProjectStep
	decodeResponse(t, stepResponse, &step)
	mainUpload := uploadWorkspaceTest(t, handler, "/api/v1/project-steps/"+string(step.ID)+"/workspace", "module", map[string]string{"src/value.txt": "main\n"}, cookie)
	if mainUpload.Code != http.StatusOK {
		t.Fatalf("main upload=%d %s", mainUpload.Code, mainUpload.Body.String())
	}
	const reviewRunnerToken = "review-runner-token"
	enrollment, err := repository.EnrollRunner(ctx, owner.ID, workspace.ID, auth.HashToken(reviewRunnerToken), "Review PC", "review-device", "darwin", "arm64", "dev", []domain.AgentRegistration{{LocalID: "review-codex", Type: "codex", DisplayName: "Review Codex", Capabilities: []byte(`{"execution":true}`), Available: true}})
	if err != nil {
		t.Fatal(err)
	}
	reviewAgentID := enrollment.Agents[0].AgentID
	if err := repository.ImportAgent(ctx, workspace.ID, reviewAgentID, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddRoomAgent(ctx, room.ID, reviewAgentID, owner.ID); err != nil {
		t.Fatal(err)
	}
	messageResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/"+string(room.ID)+"/messages", map[string]any{"body": "Update module", "plan_step_id": string(step.ID)}, cookie)
	var message domain.Message
	decodeResponse(t, messageResponse, &message)
	items, err := repository.ListRoomWorkItems(ctx, room.ID, owner.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	branchUpload := uploadWorkspaceTest(t, handler, "/api/v1/work-items/"+string(items[0].ID)+"/submit-workspace", "module", map[string]string{"src/value.txt": "reviewed\n"}, cookie)
	if branchUpload.Code != http.StatusOK {
		t.Fatalf("branch upload=%d %s", branchUpload.Code, branchUpload.Body.String())
	}
	reviewResponse := requestJSON(t, handler, http.MethodGet, "/api/v1/work-items/"+string(items[0].ID)+"/review", nil, cookie)
	if reviewResponse.Code != http.StatusOK {
		t.Fatalf("review=%d %s", reviewResponse.Code, reviewResponse.Body.String())
	}
	var review storage.WorkspaceBranchReview
	decodeResponse(t, reviewResponse, &review)
	if !review.Mergeable || review.FileCount != 1 || review.ChangedFiles[0].Path != "src/value.txt" {
		t.Fatalf("review=%+v", review)
	}
	startReview := requestJSON(t, handler, http.MethodPost, "/api/v1/work-items/"+string(items[0].ID)+"/ai-review", map[string]any{"review_agent_id": reviewAgentID, "verification_agent_id": reviewAgentID}, cookie)
	if startReview.Code != http.StatusCreated {
		t.Fatalf("start review=%d %s", startReview.Code, startReview.Body.String())
	}
	reviewTask, err := repository.ClaimNextTask(ctx, enrollment.Runner.ID)
	if err != nil || reviewTask == nil {
		t.Fatalf("claim review=%+v err=%v", reviewTask, err)
	}
	if reviewTask.ProjectWorkspace == nil || reviewTask.ProjectWorkspace.Commit != review.HeadCommit {
		t.Fatalf("review snapshot=%+v", reviewTask.ProjectWorkspace)
	}
	if err := repository.StartTask(ctx, enrollment.Runner.ID, reviewTask.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CompleteTask(ctx, enrollment.Runner.ID, reviewTask.ID, true, "审查结论：建议合并\n完成了什么：更新模块\n是否符合要求：符合\n主要风险：未发现明显风险\n实际验证：测试通过\n需要管理员决定：无", "", 0, domain.ExecutionReceipt{}); err != nil {
		t.Fatal(err)
	}
	beforeMerge, err := repository.ListRoomWorkItems(ctx, room.ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if beforeMerge[0].AIReviewStatus != domain.WorkItemAIReviewPassed {
		t.Fatalf("review result=%+v", beforeMerge[0])
	}
	merge := requestJSON(t, handler, http.MethodPost, "/api/v1/work-items/"+string(items[0].ID)+"/merge", nil, cookie)
	if merge.Code != http.StatusOK {
		t.Fatalf("merge=%d %s", merge.Code, merge.Body.String())
	}
	verificationTask, err := repository.ClaimNextTask(ctx, enrollment.Runner.ID)
	if err != nil || verificationTask == nil {
		t.Fatalf("claim verification=%+v err=%v", verificationTask, err)
	}
	if verificationTask.ProjectWorkspace == nil {
		t.Fatal("verification did not receive merged main")
	}
	if err := repository.StartTask(ctx, enrollment.Runner.ID, verificationTask.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CompleteTask(ctx, enrollment.Runner.ID, verificationTask.ID, true, "校验结论：通过\n合并结果：项目正常\n实际验证：测试通过\n遗留问题：无", "", 0, domain.ExecutionReceipt{}); err != nil {
		t.Fatal(err)
	}
	mergedItems, err := repository.ListRoomWorkItems(ctx, room.ID, owner.ID)
	if err != nil || mergedItems[0].Status != domain.WorkItemDone || mergedItems[0].VerificationStatus != domain.WorkItemVerificationPassed {
		t.Fatalf("merged item=%+v err=%v", mergedItems, err)
	}
	relative, _ := storage.WorkspaceGitDirectory(string(workspace.ID))
	output, err := exec.Command("git", "--git-dir", filepath.Join(root, relative), "show", "main:steps/"+string(step.ID)+"/src/value.txt").CombinedOutput()
	if err != nil || string(output) != "reviewed\n" {
		t.Fatalf("main file=%q err=%v", output, err)
	}
}

func TestRemoteRunnerDownloadsFrozenSharedProjectDirectory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	ctx := context.Background()
	root := t.TempDir()
	database, err := store.Open(ctx, filepath.Join(root, "agentcollab.db"))
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
	handler := httpapi.NewRouterWithConfig(authService, repository, config.Hub{StorageRoot: root})
	cookie := loginCookie(t, handler, "owner", "password-1234")
	workspaceResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces", map[string]any{"name": "Shared Project"}, cookie)
	var workspace domain.Workspace
	decodeResponse(t, workspaceResponse, &workspace)
	roomResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces/"+string(workspace.ID)+"/rooms", map[string]any{"name": "Development"}, cookie)
	var room domain.Room
	decodeResponse(t, roomResponse, &room)
	stepResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces/"+string(workspace.ID)+"/project-steps", map[string]any{"title": "Strategy", "weight": 100}, cookie)
	var step domain.ProjectStep
	decodeResponse(t, stepResponse, &step)
	mainUpload := uploadWorkspaceTest(t, handler, "/api/v1/project-steps/"+string(step.ID)+"/workspace", "red-dividend", map[string]string{"src/strategy.py": "print('shared')\n"}, cookie)
	if mainUpload.Code != http.StatusOK {
		t.Fatalf("main upload=%d %s", mainUpload.Code, mainUpload.Body.String())
	}

	const runnerToken = "remote-runner-token"
	enrollment, err := repository.EnrollRunner(ctx, owner.ID, workspace.ID, auth.HashToken(runnerToken), "Remote PC", "remote-project-device", "windows", "amd64", "dev", []domain.AgentRegistration{{LocalID: "remote-codex", Type: "codex", DisplayName: "Remote Codex", Capabilities: []byte(`{"execution":true}`), Available: true}})
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
	now := time.Now().UTC()
	task, _, err := repository.CreateTask(ctx, domain.Task{ID: "remote-project-task", RoomID: room.ID, TargetAgentID: agentID, Title: "Review strategy", Prompt: "Review the shared project", PlanStepID: &step.ID, CreatedAt: now, UpdatedAt: now}, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNextTask(ctx, enrollment.Runner.ID)
	if err != nil || claimed == nil || claimed.ProjectWorkspace == nil {
		t.Fatalf("claimed=%+v err=%v", claimed, err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/runner/tasks/"+string(task.ID)+"/project-workspace", nil)
	request.Header.Set("Authorization", "Bearer "+runnerToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("download=%d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-AgentCollab-Project-Commit") != claimed.ProjectWorkspace.Commit || response.Header().Get("X-AgentCollab-Project-Step") != string(step.ID) {
		t.Fatalf("project headers=%v", response.Header())
	}
	reader, err := zip.NewReader(strings.NewReader(response.Body.String()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := []string{}
	for _, entry := range reader.File {
		if !entry.FileInfo().IsDir() {
			files = append(files, entry.Name)
		}
	}
	if len(files) != 1 || files[0] != "project/src/strategy.py" {
		t.Fatalf("downloaded files=%v", files)
	}
}

func TestProjectStepWorkspaceCanBeUnlinkedWithoutRewritingGitHistory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	ctx := context.Background()
	root := t.TempDir()
	database, err := store.Open(ctx, filepath.Join(root, "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	authService := auth.NewService(store.NewAuthRepository(database))
	_, err = authService.CreateUser(ctx, "owner", "Owner", "password-1234", domain.UserAdmin)
	if err != nil {
		t.Fatal(err)
	}
	repository := store.NewCollabRepository(database)
	handler := httpapi.NewRouterWithConfig(authService, repository, config.Hub{StorageRoot: root})
	cookie := loginCookie(t, handler, "owner", "password-1234")
	workspaceResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces", map[string]any{"name": "Project"}, cookie)
	var workspace domain.Workspace
	decodeResponse(t, workspaceResponse, &workspace)
	stepResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces/"+string(workspace.ID)+"/project-steps", map[string]any{"title": "Module", "weight": 100}, cookie)
	var step domain.ProjectStep
	decodeResponse(t, stepResponse, &step)
	upload := uploadWorkspaceTest(t, handler, "/api/v1/project-steps/"+string(step.ID)+"/workspace", "module", map[string]string{"src/value.txt": "main\n"}, cookie)
	if upload.Code != http.StatusOK {
		t.Fatalf("upload=%d %s", upload.Code, upload.Body.String())
	}
	var uploaded domain.ProjectStep
	decodeResponse(t, upload, &uploaded)
	if uploaded.Workspace == nil {
		t.Fatal("step workspace was not recorded")
	}
	commit := uploaded.Workspace.LastCommit
	download := requestJSON(t, handler, http.MethodGet, "/api/v1/project-steps/"+string(step.ID)+"/workspace", nil, cookie)
	if download.Code != http.StatusOK || download.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("download=%d type=%q body=%s", download.Code, download.Header().Get("Content-Type"), download.Body.String())
	}
	reader, err := zip.NewReader(bytes.NewReader(download.Body.Bytes()), int64(download.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := []string{}
	for _, entry := range reader.File {
		if !entry.FileInfo().IsDir() {
			files = append(files, entry.Name)
		}
	}
	if len(files) != 1 || files[0] != "project/src/value.txt" {
		t.Fatalf("downloaded archive files=%v", files)
	}
	remove := requestJSON(t, handler, http.MethodDelete, "/api/v1/project-steps/"+string(step.ID)+"/workspace", nil, cookie)
	if remove.Code != http.StatusOK {
		t.Fatalf("remove=%d %s", remove.Code, remove.Body.String())
	}
	var unlinked domain.ProjectStep
	decodeResponse(t, remove, &unlinked)
	if unlinked.Workspace != nil {
		t.Fatalf("workspace should be unlinked: %+v", unlinked.Workspace)
	}
	missing := requestJSON(t, handler, http.MethodGet, "/api/v1/project-steps/"+string(step.ID)+"/workspace", nil, cookie)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("unlinked workspace download=%d %s", missing.Code, missing.Body.String())
	}
	// Unlinking changes the active project binding but intentionally retains
	// the immutable commit as the audit/recovery record.
	snapshot, err := storage.InspectProjectStepSnapshot(root, string(workspace.ID), string(step.ID), commit)
	if err != nil || snapshot.FileCount != 1 {
		t.Fatalf("history snapshot=%+v err=%v", snapshot, err)
	}
}

func TestProjectStepWorkspaceIsReadableByMembersButWritableOnlyByManagerOrResponsible(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	ctx := context.Background()
	root := t.TempDir()
	database, err := store.Open(ctx, filepath.Join(root, "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	authService := auth.NewService(store.NewAuthRepository(database))
	owner, err := authService.CreateUser(ctx, "shared-owner", "Owner", "password-1234", domain.UserAdmin)
	if err != nil {
		t.Fatal(err)
	}
	member, err := authService.CreateUser(ctx, "shared-member", "Member", "password-5678", domain.UserMember)
	if err != nil {
		t.Fatal(err)
	}
	repository := store.NewCollabRepository(database)
	handler := httpapi.NewRouterWithConfig(authService, repository, config.Hub{StorageRoot: root})
	ownerCookie := loginCookie(t, handler, "shared-owner", "password-1234")
	memberCookie := loginCookie(t, handler, "shared-member", "password-5678")

	workspaceResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces", map[string]any{"name": "Shared Draft"}, ownerCookie)
	var workspace domain.Workspace
	decodeResponse(t, workspaceResponse, &workspace)
	invite, err := authService.CreateInvite(ctx, owner.ID, workspace.ID, domain.WorkspaceMember, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authService.AcceptInvite(ctx, invite.Code, member.ID); err != nil {
		t.Fatal(err)
	}
	stepResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/workspaces/"+string(workspace.ID)+"/project-steps", map[string]any{"title": "Research", "weight": 100}, ownerCookie)
	var step domain.ProjectStep
	decodeResponse(t, stepResponse, &step)
	upload := uploadWorkspaceTest(t, handler, "/api/v1/project-steps/"+string(step.ID)+"/workspace", "research", map[string]string{"report.md": "shared draft\n"}, ownerCookie)
	if upload.Code != http.StatusOK {
		t.Fatalf("owner upload=%d %s", upload.Code, upload.Body.String())
	}

	download := requestJSON(t, handler, http.MethodGet, "/api/v1/project-steps/"+string(step.ID)+"/workspace", nil, memberCookie)
	if download.Code != http.StatusOK || download.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("member download=%d type=%q body=%s", download.Code, download.Header().Get("Content-Type"), download.Body.String())
	}
	forbidden := uploadWorkspaceTest(t, handler, "/api/v1/project-steps/"+string(step.ID)+"/workspace", "member-change", map[string]string{"report.md": "must not replace\n"}, memberCookie)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("ordinary member upload=%d body=%s", forbidden.Code, forbidden.Body.String())
	}

	responsibles := []domain.UserID{member.ID}
	updated, err := repository.UpdateProjectStep(ctx, step.ID, domain.ProjectStepUpdate{ResponsibleUserIDs: &responsibles}, owner.ID)
	if err != nil || len(updated.ResponsibleUserIDs) != 1 {
		t.Fatalf("assign responsible=%+v err=%v", updated, err)
	}
	responsibleUpload := uploadWorkspaceTest(t, handler, "/api/v1/project-steps/"+string(step.ID)+"/workspace", "member-change", map[string]string{"report.md": "responsible update\n"}, memberCookie)
	if responsibleUpload.Code != http.StatusOK {
		t.Fatalf("responsible upload=%d body=%s", responsibleUpload.Code, responsibleUpload.Body.String())
	}
}

func uploadWorkspaceTest(t *testing.T, handler http.Handler, path, sourceName string, files map[string]string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("source_name", sourceName)
	for relative, content := range files {
		part, err := writer.CreateFormFile("files", filepath.Base(relative))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, content); err != nil {
			t.Fatal(err)
		}
		_ = writer.WriteField("paths", relative)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
