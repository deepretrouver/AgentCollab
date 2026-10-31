package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
)

func TestCancelTasksForRootStopsQueuedAndRunningWork(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	owner := domain.User{ID: "user-owner", Username: "owner", DisplayName: "Owner", PasswordHash: "x", Role: domain.UserAdmin, CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "user-member", Username: "member", DisplayName: "Member", PasswordHash: "x", Role: domain.UserMember, CreatedAt: now, UpdatedAt: now}
	authRepository := NewAuthRepository(database)
	for _, user := range []domain.User{owner, member} {
		if err := authRepository.CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}

	repository := NewCollabRepository(database)
	workspace := domain.Workspace{ID: "workspace-cancel", Name: "Cancel", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateWorkspace(ctx, workspace, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)`, workspace.ID, member.ID, formatTime(now))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	room := domain.Room{ID: "room-cancel", WorkspaceID: workspace.ID, Name: "General", CreatedAt: now}
	if err := repository.CreateRoom(ctx, room, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddRoomMember(ctx, room.ID, owner.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	enrollment, err := repository.EnrollRunner(ctx, owner.ID, workspace.ID, "runner-token", "Owner Mac", "owner-installation", "darwin", "arm64", "test", []domain.AgentRegistration{{LocalID: "codex", Type: "codex", DisplayName: "Codex", Capabilities: []byte(`{"execution":true}`), Available: true}})
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

	root, err := repository.CreateMessage(ctx, domain.Message{ID: "message-cancel", RoomID: room.ID, SenderKind: domain.MessageSenderUser, SenderID: string(owner.ID), Kind: domain.MessageChat, Body: "@owner Codex 请做一份长报告", MentionAgentIDs: []domain.AgentID{agentID}, CreatedAt: now}, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := repository.ListRoomTasks(ctx, room.ID, owner.ID)
	if err != nil || len(queued) != 1 || queued[0].State != domain.TaskQueued {
		t.Fatalf("queued tasks=%+v err=%v", queued, err)
	}
	items, err := repository.ListRoomWorkItems(ctx, room.ID, owner.ID)
	if err != nil || len(items) != 1 || items[0].RootMessageID == nil || *items[0].RootMessageID != root.ID || items[0].TaskCount != 1 || items[0].Status != domain.WorkItemInProgress {
		t.Fatalf("work items=%+v err=%v", items, err)
	}
	claimed, err := repository.ClaimNextTask(ctx, enrollment.Runner.ID)
	if err != nil || claimed == nil {
		t.Fatalf("claim task=%+v err=%v", claimed, err)
	}
	if err := repository.StartTask(ctx, enrollment.Runner.ID, claimed.ID); err != nil {
		t.Fatal(err)
	}
	if recovered, err := repository.RecoverInterruptedTasks(ctx, enrollment.Runner.ID); err != nil || recovered != 1 {
		t.Fatalf("recover interrupted task=%d err=%v", recovered, err)
	}
	claimed, err = repository.ClaimNextTask(ctx, enrollment.Runner.ID)
	if err != nil || claimed == nil {
		t.Fatalf("reclaim recovered task=%+v err=%v", claimed, err)
	}
	if err := repository.StartTask(ctx, enrollment.Runner.ID, claimed.ID); err != nil {
		t.Fatal(err)
	}

	if _, _, err := repository.CancelTasksForRoot(ctx, room.ID, root.ID, member.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("member stopped another user's topic: %v", err)
	}
	cancelled, notice, err := repository.CancelTasksForRoot(ctx, room.ID, root.ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cancelled) != 1 || cancelled[0].ID != claimed.ID || cancelled[0].State != domain.TaskCancelled {
		t.Fatalf("cancelled tasks=%+v", cancelled)
	}
	if notice.ID == "" || notice.SenderKind != domain.MessageSenderSystem || notice.RootMessageID == nil || *notice.RootMessageID != root.ID {
		t.Fatalf("cancellation notice=%+v", notice)
	}
	requested, err := repository.TaskCancellationRequested(ctx, enrollment.Runner.ID, claimed.ID)
	if err != nil || !requested {
		t.Fatalf("Runner cancellation requested=%v err=%v", requested, err)
	}
	remaining, err := repository.ListRoomTasks(ctx, room.ID, owner.ID)
	if err != nil || len(remaining) != 1 || remaining[0].State != domain.TaskCancelled {
		t.Fatalf("remaining tasks=%+v err=%v", remaining, err)
	}
	if next, err := repository.ClaimNextTask(ctx, enrollment.Runner.ID); err != nil || next != nil {
		t.Fatalf("cancelled task was claimed again: task=%+v err=%v", next, err)
	}
	items, err = repository.ListRoomWorkItems(ctx, room.ID, owner.ID)
	if err != nil || len(items) != 1 || items[0].Status != domain.WorkItemCancelled {
		t.Fatalf("cancelled work items=%+v err=%v", items, err)
	}
}
