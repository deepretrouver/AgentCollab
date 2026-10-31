package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
)

func TestWeightedProjectPlanProgressAndConversationAssignment(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	owner := domain.User{ID: "project_owner", Username: "project-owner", DisplayName: "Project Owner", PasswordHash: "x", Role: domain.UserAdmin, CreatedAt: now, UpdatedAt: now}
	if err := NewAuthRepository(database).CreateUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	repository := NewCollabRepository(database)
	workspace := domain.Workspace{ID: "project_workspace", Name: "Project", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateWorkspace(ctx, workspace, owner.ID); err != nil {
		t.Fatal(err)
	}
	room := domain.Room{ID: "project_room", WorkspaceID: workspace.ID, Name: "Delivery", CreatedAt: now}
	if err := repository.CreateRoom(ctx, room, owner.ID); err != nil {
		t.Fatal(err)
	}

	objective := "完成投研项目并形成可验收结论"
	plan, err := repository.UpdateWorkspaceProjectPlan(ctx, workspace.ID, domain.ProjectPlanUpdate{Objective: &objective}, owner.ID)
	if err != nil || plan.Objective != objective || !plan.CanManage {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	first, err := repository.CreateProjectStep(ctx, workspace.ID, domain.ProjectStepCreate{Title: "确定范围", Weight: 30}, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.CreateProjectStep(ctx, workspace.ID, domain.ProjectStepCreate{Title: "形成结论", Weight: 70}, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	childA, err := repository.CreateProjectStep(ctx, workspace.ID, domain.ProjectStepCreate{ParentStepID: &second.ID, Title: "证据分析", Weight: 40}, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateProjectStep(ctx, workspace.ID, domain.ProjectStepCreate{ParentStepID: &second.ID, Title: "报告验收", Weight: 60}, owner.ID); err != nil {
		t.Fatal(err)
	}
	done := domain.ProjectStepDone
	if _, err := repository.UpdateProjectStep(ctx, first.ID, domain.ProjectStepUpdate{Status: &done}, owner.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpdateProjectStep(ctx, childA.ID, domain.ProjectStepUpdate{Status: &done}, owner.ID); err != nil {
		t.Fatal(err)
	}
	plan, err = repository.GetWorkspaceProjectPlan(ctx, workspace.ID, owner.ID)
	if err != nil || plan.Progress != 58 {
		t.Fatalf("weighted progress=%d err=%v steps=%+v", plan.Progress, err, plan.Steps)
	}

	metadata, _ := json.Marshal(map[string]any{"plan_step_id": childA.ID})
	message := domain.Message{ID: "project_message", RoomID: room.ID, SenderKind: domain.MessageSenderUser, SenderID: string(owner.ID), Kind: domain.MessageChat, Body: "整理证据清单", Metadata: metadata, CreatedAt: now}
	if _, err := repository.CreateMessage(ctx, message, owner.ID); err != nil {
		t.Fatal(err)
	}
	items, err := repository.ListRoomWorkItems(ctx, room.ID, owner.ID)
	if err != nil || len(items) != 1 || items[0].PlanStepID == nil || *items[0].PlanStepID != childA.ID {
		t.Fatalf("assigned items=%+v err=%v", items, err)
	}
	if items[0].GitBranch != "" || items[0].MergeStatus != domain.WorkItemMergeNone {
		t.Fatalf("discussion should not create a branch before a directory is submitted: %+v", items[0])
	}
	plan, err = repository.GetWorkspaceProjectPlan(ctx, workspace.ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range plan.Steps {
		if step.ID == childA.ID && step.LinkedItemCount != 1 {
			t.Fatalf("linked item count=%d", step.LinkedItemCount)
		}
	}
}

func TestOnlyProjectManagersCanChangePlan(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	owner := domain.User{ID: "owner_plan_roles", Username: "owner-plan-roles", DisplayName: "Owner", PasswordHash: "x", Role: domain.UserAdmin, CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "member_plan_roles", Username: "member-plan-roles", DisplayName: "Member", PasswordHash: "x", Role: domain.UserMember, CreatedAt: now, UpdatedAt: now}
	authRepository := NewAuthRepository(database)
	for _, user := range []domain.User{owner, member} {
		if err := authRepository.CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	repository := NewCollabRepository(database)
	workspace := domain.Workspace{ID: "workspace_plan_roles", Name: "Roles", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateWorkspace(ctx, workspace, owner.ID); err != nil {
		t.Fatal(err)
	}
	room := domain.Room{ID: "room_plan_roles", WorkspaceID: workspace.ID, Name: "Roles", CreatedAt: now}
	if err := repository.CreateRoom(ctx, room, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)`, workspace.ID, member.ID, formatTime(now))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddRoomMember(ctx, room.ID, owner.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	objective := "member cannot rewrite this"
	if _, err := repository.UpdateWorkspaceProjectPlan(ctx, workspace.ID, domain.ProjectPlanUpdate{Objective: &objective}, member.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("member changed project plan: %v", err)
	}
}

func TestProjectStepHumanResponsiblesCanBeReplacedWithoutDeletingMembers(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	owner := domain.User{ID: "owner_responsibles", Username: "owner-responsibles", DisplayName: "Owner", PasswordHash: "x", Role: domain.UserAdmin, CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "member_responsibles", Username: "member-responsibles", DisplayName: "Member", PasswordHash: "x", Role: domain.UserMember, CreatedAt: now, UpdatedAt: now}
	authRepository := NewAuthRepository(database)
	for _, user := range []domain.User{owner, member} {
		if err := authRepository.CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	repository := NewCollabRepository(database)
	workspace := domain.Workspace{ID: "workspace_responsibles", Name: "Responsibles", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateWorkspace(ctx, workspace, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)`, workspace.ID, member.ID, formatTime(now))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	step, err := repository.CreateProjectStep(ctx, workspace.ID, domain.ProjectStepCreate{Title: "负责人测试", Weight: 100, ResponsibleUserIDs: []domain.UserID{member.ID}}, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(step.ResponsibleUserIDs) != 1 || step.ResponsibleUserIDs[0] != member.ID {
		t.Fatalf("unexpected responsible assignments: %+v", step)
	}
	empty := []domain.UserID{}
	step, err = repository.UpdateProjectStep(ctx, step.ID, domain.ProjectStepUpdate{ResponsibleUserIDs: &empty}, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(step.ResponsibleUserIDs) != 0 {
		t.Fatalf("responsibles were not cleared: %+v", step)
	}
	var memberStillExists int
	if err := database.Read(ctx, func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspace.ID, member.ID).Scan(&memberStillExists)
	}); err != nil || memberStillExists != 1 {
		t.Fatalf("workspace member was affected: exists=%d err=%v", memberStillExists, err)
	}
}
