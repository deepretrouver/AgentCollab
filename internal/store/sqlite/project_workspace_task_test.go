package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"agentcollab.local/agentcollab/internal/domain"
)

func TestResponsibleAgentCanUseOnlyItsOwnersBoundLocalStepDirectory(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	owner := domain.User{ID: "local_owner", Username: "local-owner", DisplayName: "Local Owner", PasswordHash: "x", Role: domain.UserAdmin, CreatedAt: now, UpdatedAt: now}
	if err := NewAuthRepository(database).CreateUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	repository := NewCollabRepository(database)
	workspace := domain.Workspace{ID: "local_workspace", Name: "Local", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateWorkspace(ctx, workspace, owner.ID); err != nil {
		t.Fatal(err)
	}
	room := domain.Room{ID: "local_room", WorkspaceID: workspace.ID, Name: "Progress", CreatedAt: now}
	if err := repository.CreateRoom(ctx, room, owner.ID); err != nil {
		t.Fatal(err)
	}
	enrollment, err := repository.EnrollRunner(ctx, owner.ID, workspace.ID, "local-token", "Owner Mac", "local-device-key", "darwin", "arm64", "dev", []domain.AgentRegistration{
		{LocalID: "responsible", Type: "codex", DisplayName: "Responsible Codex", Capabilities: []byte(`{"execution":true}`), Available: true},
		{LocalID: "unassigned", Type: "opencode", DisplayName: "Unassigned OpenCode", Capabilities: []byte(`{"execution":true}`), Available: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range enrollment.Agents {
		if err := repository.ImportAgent(ctx, workspace.ID, agent.AgentID, owner.ID); err != nil {
			t.Fatal(err)
		}
		if err := repository.AddRoomAgent(ctx, room.ID, agent.AgentID, owner.ID); err != nil {
			t.Fatal(err)
		}
	}
	responsibleID, unassignedID := enrollment.Agents[0].AgentID, enrollment.Agents[1].AgentID
	step, err := repository.CreateProjectStep(ctx, workspace.ID, domain.ProjectStepCreate{Title: "Implementation", Weight: 100, ResponsibleUserIDs: []domain.UserID{owner.ID}, ResponsibleAgentIDs: []domain.AgentID{responsibleID}}, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	step, err = repository.SaveProjectStepLocalBinding(ctx, step.ID, domain.ProjectStepLocalBindingUpdate{RunnerID: enrollment.Runner.ID, DeviceID: enrollment.Device.ID, DirectoryLabel: "live-project"}, owner.ID)
	if err != nil || len(step.LocalBindings) != 1 {
		t.Fatalf("binding=%+v err=%v", step.LocalBindings, err)
	}
	bad := domain.Task{ID: "bad-local-task", RoomID: room.ID, TargetAgentID: unassignedID, Title: "Inspect", Prompt: "status", PlanStepID: &step.ID, ExecutionSource: domain.TaskExecutionResponsibleLocal, CreatedAt: now, UpdatedAt: now}
	if _, _, err := repository.CreateTask(ctx, bad, owner.ID); err == nil {
		t.Fatal("unassigned Agent borrowed a responsible local directory")
	}
	task := domain.Task{ID: "good-local-task", RoomID: room.ID, TargetAgentID: responsibleID, Title: "Inspect", Prompt: "status", PlanStepID: &step.ID, ExecutionSource: domain.TaskExecutionResponsibleLocal, CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)}
	if _, _, err := repository.CreateTask(ctx, task, owner.ID); err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNextTask(ctx, enrollment.Runner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.LocalWorkspace == nil || claimed.ProjectWorkspace != nil || claimed.LocalWorkspace.StepID != step.ID || claimed.LocalWorkspace.DirectoryLabel != "live-project" {
		t.Fatalf("unexpected local claim: %+v", claimed)
	}
}

func TestOfficialExecutionRequiresSharedProjectWorkspace(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	owner := domain.User{ID: "official_owner", Username: "official-owner", DisplayName: "Owner", PasswordHash: "x", Role: domain.UserAdmin, CreatedAt: now, UpdatedAt: now}
	if err := NewAuthRepository(database).CreateUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	repository := NewCollabRepository(database)
	workspace := domain.Workspace{ID: "official_workspace", Name: "Official", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateWorkspace(ctx, workspace, owner.ID); err != nil {
		t.Fatal(err)
	}
	room := domain.Room{ID: "official_room", WorkspaceID: workspace.ID, Name: "Research", CreatedAt: now}
	if err := repository.CreateRoom(ctx, room, owner.ID); err != nil {
		t.Fatal(err)
	}
	enrollment, err := repository.EnrollRunner(ctx, owner.ID, workspace.ID, "official-token", "Owner Mac", "official-device", "darwin", "arm64", "dev", []domain.AgentRegistration{{LocalID: "codex", Type: "codex", DisplayName: "Codex", Capabilities: []byte(`{"execution":true}`), Available: true}})
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
	step, err := repository.CreateProjectStep(ctx, workspace.ID, domain.ProjectStepCreate{Title: "Allocation", Weight: 100}, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	task := domain.Task{ID: "official-without-workspace", RoomID: room.ID, TargetAgentID: agentID, Title: "Analyze", Prompt: "analyze", PlanStepID: &step.ID, ExecutionSource: domain.TaskExecutionOfficial, CreatedAt: now, UpdatedAt: now}
	if _, _, err := repository.CreateTask(ctx, task, owner.ID); err == nil {
		t.Fatal("official execution was accepted without a shared project workspace")
	}
}

func TestClaimedTaskFreezesSharedProjectWorkspaceForAnyRunner(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	owner := domain.User{ID: "project_share_owner", Username: "project-share-owner", DisplayName: "Owner", PasswordHash: "x", Role: domain.UserAdmin, CreatedAt: now, UpdatedAt: now}
	if err := NewAuthRepository(database).CreateUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	repository := NewCollabRepository(database)
	workspace := domain.Workspace{ID: "project_share_workspace", Name: "Shared Project", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateWorkspace(ctx, workspace, owner.ID); err != nil {
		t.Fatal(err)
	}
	room := domain.Room{ID: "project_share_room", WorkspaceID: workspace.ID, Name: "Development", CreatedAt: now}
	if err := repository.CreateRoom(ctx, room, owner.ID); err != nil {
		t.Fatal(err)
	}
	enrollment, err := repository.EnrollRunner(ctx, owner.ID, workspace.ID, "project-share-token", "Remote PC", "project-share-device", "windows", "amd64", "dev", []domain.AgentRegistration{{LocalID: "remote-codex", Type: "codex", DisplayName: "Remote Codex", Capabilities: []byte(`{"execution":true}`), Available: true}})
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
	step, err := repository.CreateProjectStep(ctx, workspace.ID, domain.ProjectStepCreate{Title: "Implementation", Weight: 100}, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SaveProjectStepWorkspace(ctx, step.ID, "red-dividend", 3, 1234, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", owner.ID); err != nil {
		t.Fatal(err)
	}
	task, _, err := repository.CreateTask(ctx, domain.Task{ID: "project_share_task", RoomID: room.ID, TargetAgentID: agentID, Title: "Review", Prompt: "Review project", PlanStepID: &step.ID, CreatedAt: now, UpdatedAt: now}, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNextTask(ctx, enrollment.Runner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || task.WorkItemID == nil || claimed.ProjectWorkspace == nil {
		t.Fatalf("claimed task lacks shared project: task=%+v claimed=%+v", task, claimed)
	}
	if claimed.ProjectWorkspace.WorkspaceID != workspace.ID || claimed.ProjectWorkspace.StepID != step.ID || claimed.ProjectWorkspace.Commit != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || claimed.ProjectWorkspace.FileCount != 3 || claimed.ProjectWorkspace.TotalBytes != 1234 {
		t.Fatalf("unexpected project snapshot: %+v", claimed.ProjectWorkspace)
	}
	if claimed.ProjectContext == nil || claimed.ProjectContext.StepID != step.ID || claimed.ProjectContext.StepTitle != "Implementation" {
		t.Fatalf("claimed task lacks shared project memory: %+v", claimed.ProjectContext)
	}
	// Updating the step after dispatch must not change this task's frozen copy.
	if _, err := repository.SaveProjectStepWorkspace(ctx, step.ID, "red-dividend-v2", 4, 2048, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", owner.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.ProjectWorkspaceForRunnerTask(ctx, claimed.ID, enrollment.Runner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Commit != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || snapshot.SourceName != "red-dividend" || snapshot.FileCount != 3 || snapshot.TotalBytes != 1234 {
		t.Fatalf("task snapshot drifted after project update: %+v", snapshot)
	}
	var frozen string
	if err := database.Read(ctx, func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `SELECT project_workspace_commit FROM tasks WHERE id = ?`, claimed.ID).Scan(&frozen)
	}); err != nil || frozen != snapshot.Commit {
		t.Fatalf("frozen commit=%q err=%v", frozen, err)
	}
}
