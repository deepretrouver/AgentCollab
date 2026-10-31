package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
)

func TestAgentOwnershipVisibilityInvocationAndStableDeviceIdentity(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	owner := domain.User{ID: "user_owner", Username: "owner", DisplayName: "Owner", PasswordHash: "x", Role: domain.UserAdmin, CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "user_alice", Username: "alice", DisplayName: "Alice", PasswordHash: "x", Role: domain.UserMember, CreatedAt: now, UpdatedAt: now}
	authRepository := NewAuthRepository(database)
	for _, user := range []domain.User{owner, member} {
		if err := authRepository.CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	repository := NewCollabRepository(database)
	workspace := domain.Workspace{ID: "ws_permissions", Name: "Permissions", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateWorkspace(ctx, workspace, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)`, workspace.ID, member.ID, formatTime(now))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	room := domain.Room{ID: "room_permissions", WorkspaceID: workspace.ID, Name: "Review", CreatedAt: now}
	if err := repository.CreateRoom(ctx, room, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddRoomMember(ctx, room.ID, owner.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	ownerCapabilities := []byte(`{"execution":true,"skills":[{"name":"stock-review"},{"name":"private-ledger"}],"resources":[{"name":"owner-files","access":"read-write"}]}`)
	first, err := repository.EnrollRunner(ctx, owner.ID, workspace.ID, "token-owner-1", "Old Device Name", "installation-owner", "windows", "amd64", "0.2.0-permissions", []domain.AgentRegistration{{LocalID: "scan-codex", Type: "codex", DisplayName: "Codex Local", Capabilities: ownerCapabilities, Available: true}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.EnrollRunner(ctx, owner.ID, workspace.ID, "token-owner-2", "Renamed Device", "installation-owner", "windows", "amd64", "0.2.0-permissions", []domain.AgentRegistration{{LocalID: "scan-codex", Type: "codex", DisplayName: "Codex Local", Capabilities: ownerCapabilities, Available: true}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Device.ID != second.Device.ID || first.Runner.ID != second.Runner.ID || first.Agents[0].AgentID != second.Agents[0].AgentID {
		t.Fatalf("stable enrollment duplicated identity: first=%+v second=%+v", first, second)
	}
	ownerAgentID := second.Agents[0].AgentID
	if err := repository.ImportAgent(ctx, workspace.ID, ownerAgentID, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddRoomAgent(ctx, room.ID, ownerAgentID, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateAgentAccessPolicy(ctx, workspace.ID, domain.AgentAccessPolicy{AgentID: ownerAgentID, Visibility: domain.AgentVisibilityPrivate, InvocationScope: domain.AgentInvocationOwnerOnly}, owner.ID); err != nil {
		t.Fatal(err)
	}
	privateView, err := repository.ListAgents(ctx, workspace.ID, member.ID)
	if err != nil || len(privateView) != 0 {
		t.Fatalf("private Agent visible to member: %+v, %v", privateView, err)
	}
	if err := repository.UpdateAgentAccessPolicy(ctx, workspace.ID, domain.AgentAccessPolicy{AgentID: ownerAgentID, Visibility: domain.AgentVisibilityWorkspace, InvocationScope: domain.AgentInvocationOwnerOnly}, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddRoomAgent(ctx, room.ID, ownerAgentID, owner.ID); err != nil {
		t.Fatal(err)
	}

	memberEnrollment, err := repository.EnrollRunner(ctx, member.ID, workspace.ID, "token-member", "Alice PC", "installation-alice", "linux", "amd64", "test", []domain.AgentRegistration{{LocalID: "scan-hermes", Type: "hermes", DisplayName: "Hermes Local", Capabilities: []byte(`{"execution":true}`), Available: true}})
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := repository.ListAgentCandidates(ctx, workspace.ID, member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].AgentID != memberEnrollment.Agents[0].AgentID || candidates[0].DisplayName != "Alice Hermes" {
		t.Fatalf("member candidates = %+v, want only alice Hermes", candidates)
	}
	if err := database.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE runners SET last_seen_at = ? WHERE id = ?`, formatTime(now.Add(-time.Hour)), memberEnrollment.Runner.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	staleCandidates, err := repository.ListAgentCandidates(ctx, workspace.ID, member.ID)
	if err != nil || len(staleCandidates) != 0 {
		t.Fatalf("stale Runner leaked into scan candidates: %+v, %v", staleCandidates, err)
	}

	ownerView, err := repository.ListRoomAgents(ctx, room.ID, owner.ID)
	if err != nil || len(ownerView) != 1 {
		t.Fatalf("owner room agents = %+v, %v", ownerView, err)
	}
	if ownerView[0].DisplayName != "Owner Codex" || !ownerView[0].CanManage || !ownerView[0].CanInvoke || len(ownerView[0].Skills) != 2 || len(ownerView[0].Resources) != 1 {
		t.Fatalf("owner Agent view = %+v", ownerView[0])
	}
	memberView, err := repository.ListRoomAgents(ctx, room.ID, member.ID)
	if err != nil || len(memberView) != 1 {
		t.Fatalf("member room agents = %+v, %v", memberView, err)
	}
	if memberView[0].CanManage || memberView[0].CanInvoke || len(memberView[0].Skills) != 0 || len(memberView[0].Resources) != 0 {
		t.Fatalf("unshared Agent leaked to member: %+v", memberView[0])
	}
	if err := repository.UpdateRoomAgentSettings(ctx, domain.RoomAgentSettings{RoomID: room.ID, AgentID: ownerAgentID, ResponseMode: domain.AgentResponseHumanAll, MaxAutoTurns: 4}, owner.ID); err != nil {
		t.Fatal(err)
	}
	memberMessage := domain.Message{ID: "message_owner_only", RoomID: room.ID, SenderKind: domain.MessageSenderUser, SenderID: string(member.ID), Kind: domain.MessageChat, Body: "普通成员消息", CreatedAt: now}
	if _, err := repository.CreateMessage(ctx, memberMessage, member.ID); err != nil {
		t.Fatal(err)
	}
	routedTasks, err := repository.ListRoomTasks(ctx, room.ID, member.ID)
	if err != nil || len(routedTasks) != 0 {
		t.Fatalf("owner-only Agent auto-replied to another member: %+v, %v", routedTasks, err)
	}

	settings := domain.RoomAgentSettings{RoomID: room.ID, AgentID: ownerAgentID, ResponseMode: domain.AgentResponseHumanAll, MaxAutoTurns: 4}
	if err := repository.UpdateRoomAgentSettings(ctx, settings, member.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("member changed another user's Agent settings: %v", err)
	}
	task := domain.Task{ID: "task_denied", RoomID: room.ID, TargetAgentID: ownerAgentID, Title: "Review", Prompt: "Review it", CreatedAt: now, UpdatedAt: now}
	if _, _, err := repository.CreateTask(ctx, task, member.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("member invoked owner-only Agent: %v", err)
	}
	if err := repository.UpdateAgentAccessPolicy(ctx, workspace.ID, domain.AgentAccessPolicy{AgentID: ownerAgentID, Visibility: domain.AgentVisibilityWorkspace, InvocationScope: domain.AgentInvocationRoomMembers, SharedSkills: []string{"stock-review"}}, member.ID); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("member changed another user's policy: %v", err)
	}
	if err := repository.UpdateAgentAccessPolicy(ctx, workspace.ID, domain.AgentAccessPolicy{AgentID: ownerAgentID, Visibility: domain.AgentVisibilityWorkspace, InvocationScope: domain.AgentInvocationRoomMembers, SharedSkills: []string{"stock-review", "not-installed"}}, owner.ID); err != nil {
		t.Fatal(err)
	}
	memberView, err = repository.ListRoomAgents(ctx, room.ID, member.ID)
	if err != nil || len(memberView) != 1 || !memberView[0].CanInvoke || !reflect.DeepEqual(memberView[0].SharedSkills, []string{"stock-review"}) || len(memberView[0].Skills) != 1 || memberView[0].Skills[0].Name != "stock-review" || len(memberView[0].Resources) != 0 {
		t.Fatalf("shared member view = %+v, %v", memberView, err)
	}

	task.ID = "task_allowed"
	created, taskMessage, err := repository.CreateTask(ctx, task, member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if created.RootMessageID == nil || created.SourceMessageID == nil {
		t.Fatalf("structured task is not linked to its root/source message: %+v", created)
	}
	if created.WorkItemID == nil {
		t.Fatalf("structured task is not linked to a project work item: %+v", created)
	}
	if taskMessage.Kind != domain.MessageChat || taskMessage.Body != task.Prompt || !reflect.DeepEqual(taskMessage.MentionAgentIDs, []domain.AgentID{ownerAgentID}) {
		t.Fatalf("structured task did not create a visible topic root: %+v", taskMessage)
	}
	claimed, err := repository.ClaimNextTask(ctx, second.Runner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.CapabilityScope != "shared_only" || !reflect.DeepEqual(claimed.AuthorizedSkills, []string{"stock-review"}) {
		t.Fatalf("claimed shared task = %+v", claimed)
	}
	if err := repository.StartTask(ctx, second.Runner.ID, claimed.ID); err != nil {
		t.Fatal(err)
	}
	artifact := domain.Attachment{ID: "attachment-delivery", Filename: "review.md", ContentType: "text/markdown", SizeBytes: 9, SHA256: "abc123", RelativePath: "artifacts/tasks/task_allowed/review.md", CreatedAt: now}
	if _, err := repository.CreateTaskArtifact(ctx, claimed.ID, second.Runner.ID, artifact); err != nil {
		t.Fatalf("upload task artifact: %v", err)
	}
	firstDelivery, err := repository.CompleteTask(ctx, second.Runner.ID, claimed.ID, true, "REVIEW_OK", "", 0, domain.ExecutionReceipt{})
	if err != nil {
		t.Fatal(err)
	}
	replayedDelivery, err := repository.CompleteTask(ctx, second.Runner.ID, claimed.ID, true, "REVIEW_OK", "", 0, domain.ExecutionReceipt{})
	if err != nil {
		t.Fatalf("idempotent completion retry failed: %v", err)
	}
	if replayedDelivery.ID != firstDelivery.ID {
		t.Fatalf("completion retry created a duplicate message: first=%s replay=%s", firstDelivery.ID, replayedDelivery.ID)
	}
	if len(firstDelivery.Attachments) != 1 || firstDelivery.Attachments[0].Filename != "review.md" || firstDelivery.Attachments[0].MessageID == nil || *firstDelivery.Attachments[0].MessageID != firstDelivery.ID {
		t.Fatalf("task artifact was not attached to delivery: %+v", firstDelivery.Attachments)
	}
	items, err := repository.ListRoomWorkItems(ctx, room.ID, member.ID)
	if err != nil || len(items) != 1 || items[0].Status != domain.WorkItemReview || items[0].DeliveredTaskCount != 1 {
		t.Fatalf("delivered work item=%+v err=%v", items, err)
	}
	if _, err := repository.UpdateWorkItem(ctx, *created.WorkItemID, domain.WorkItemUpdate{Status: workItemStatusPtr(domain.WorkItemDone)}, member.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("ordinary member accepted a delivery: %v", err)
	}
	accepted, err := repository.UpdateWorkItem(ctx, *created.WorkItemID, domain.WorkItemUpdate{Status: workItemStatusPtr(domain.WorkItemDone)}, owner.ID)
	if err != nil || accepted.Status != domain.WorkItemDone || accepted.AcceptedAt == nil {
		t.Fatalf("accepted work item=%+v err=%v", accepted, err)
	}
	if err := database.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE runners SET version = 'dev' WHERE id = ?`, second.Runner.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	task.ID = "task_development_client_allowed"
	if _, _, err := repository.CreateTask(ctx, task, member.ID); err != nil {
		t.Fatalf("development Client rejected a cross-owner task: %v", err)
	}
}

func workItemStatusPtr(value domain.WorkItemStatus) *domain.WorkItemStatus { return &value }

func TestRunnerAgentLocalKeySurvivesDisplayNameChange(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	owner := domain.User{ID: "user_owner", Username: "owner", DisplayName: "Owner", PasswordHash: "x", Role: domain.UserAdmin, CreatedAt: now, UpdatedAt: now}
	if err := NewAuthRepository(database).CreateUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	repository := NewCollabRepository(database)
	workspace := domain.Workspace{ID: "ws_local_key", Name: "Stable Agent", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateWorkspace(ctx, workspace, owner.ID); err != nil {
		t.Fatal(err)
	}
	first, err := repository.EnrollRunner(ctx, owner.ID, workspace.ID, "token-1", "Mac", "stable-machine", "darwin", "arm64", "test", []domain.AgentRegistration{{LocalID: "desk-codex", Type: "codex", DisplayName: "Research Codex", Capabilities: []byte(`{"execution":true}`), Available: true}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.EnrollRunner(ctx, owner.ID, workspace.ID, "token-2", "Renamed Mac", "stable-machine", "darwin", "arm64", "test", []domain.AgentRegistration{{LocalID: "desk-codex", Type: "codex", DisplayName: "Renamed Research Codex", Capabilities: []byte(`{"execution":true}`), Available: true}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Agents[0].AgentID != second.Agents[0].AgentID {
		t.Fatalf("display-name change duplicated Agent: first=%s second=%s", first.Agents[0].AgentID, second.Agents[0].AgentID)
	}
	var count int
	var localKey, displayName string
	if err := database.Read(ctx, func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `SELECT COUNT(*), MIN(local_key), MIN(display_name) FROM agents WHERE runner_id = ?`, second.Runner.ID).Scan(&count, &localKey, &displayName)
	}); err != nil {
		t.Fatal(err)
	}
	if count != 1 || localKey != "desk-codex" || displayName != "Renamed Research Codex" {
		t.Fatalf("stable Agent row count=%d local_key=%q display_name=%q", count, localKey, displayName)
	}
}

func TestUserDisplayNameRefreshesOnlyManagedAgentNames(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	owner := domain.User{ID: "profile-owner", Username: "owner", DisplayName: "Owner", PasswordHash: "x", Role: domain.UserAdmin, CreatedAt: now, UpdatedAt: now}
	authRepository := NewAuthRepository(database)
	if err := authRepository.CreateUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	repository := NewCollabRepository(database)
	workspace := domain.Workspace{ID: "profile-workspace", Name: "Profile", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateWorkspace(ctx, workspace, owner.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.EnrollRunner(ctx, owner.ID, workspace.ID, "profile-token", "Owner Mac", "profile-machine", "darwin", "arm64", "dev", []domain.AgentRegistration{
		{LocalID: "scan-codex", Type: "codex", DisplayName: "Codex Local", Available: true},
		{LocalID: "research-codex", Type: "codex", DisplayName: "Research Codex", Available: true},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := authRepository.UpdateUserProfile(ctx, owner.ID, auth.ProfileInput{DisplayName: "郭昱科"}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	if err := database.Read(ctx, func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `SELECT local_key, source_display_name FROM agents ORDER BY local_key`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var localKey, displayName string
			if err := rows.Scan(&localKey, &displayName); err != nil {
				return err
			}
			got[localKey] = displayName
		}
		return rows.Err()
	}); err != nil {
		t.Fatal(err)
	}
	if got["scan-codex"] != "郭昱科 Codex" || got["research-codex"] != "Research Codex" {
		t.Fatalf("display names after profile update = %#v", got)
	}
}

func TestWorkspaceCanReuseExistingLocalAgentWithoutReenrollingRunner(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	owner := domain.User{ID: "user_owner", Username: "owner", DisplayName: "Owner", PasswordHash: "x", Role: domain.UserAdmin, CreatedAt: now, UpdatedAt: now}
	if err := NewAuthRepository(database).CreateUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	repository := NewCollabRepository(database)
	first := domain.Workspace{ID: "ws_first", Name: "Local", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	second := domain.Workspace{ID: "ws_second", Name: "Research", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	for _, workspace := range []domain.Workspace{first, second} {
		if err := repository.CreateWorkspace(ctx, workspace, owner.ID); err != nil {
			t.Fatal(err)
		}
	}
	room := domain.Room{ID: "room_second", WorkspaceID: second.ID, Name: "General", CreatedAt: now}
	if err := repository.CreateRoom(ctx, room, owner.ID); err != nil {
		t.Fatal(err)
	}

	enrollment, err := repository.EnrollRunner(ctx, owner.ID, first.ID, "token-local", "Owner Mac", "stable-owner-mac", "darwin", "arm64", "0.2.0", []domain.AgentRegistration{{
		LocalID: "scan-codex", Type: "codex", DisplayName: "Codex Local", Available: true, Capabilities: []byte(`{"execution":true}`),
	}})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := enrollment.Agents[0].AgentID
	if err := repository.ImportAgent(ctx, first.ID, sourceID, owner.ID); err != nil {
		t.Fatal(err)
	}

	candidates, err := repository.ListAgentCandidates(ctx, second.ID, owner.ID)
	if err != nil || len(candidates) != 1 || candidates[0].AgentID != sourceID {
		t.Fatalf("cross-workspace candidates=%+v, err=%v", candidates, err)
	}
	if err := repository.ImportAgent(ctx, second.ID, sourceID, owner.ID); err != nil {
		t.Fatal(err)
	}
	shared, err := repository.ListAgents(ctx, second.ID, owner.ID)
	if err != nil || len(shared) != 1 || shared[0].AgentID == sourceID || shared[0].DisplayName != "Owner Codex" || shared[0].Status != domain.AgentOnline {
		t.Fatalf("reused workspace Agent=%+v, err=%v", shared, err)
	}
	cloneID := shared[0].AgentID
	if err := repository.AddRoomAgent(ctx, room.ID, cloneID, owner.ID); err != nil {
		t.Fatal(err)
	}
	created, _, err := repository.CreateTask(ctx, domain.Task{ID: "task_reused", RoomID: room.ID, TargetAgentID: cloneID, Title: "Reuse", Prompt: "Return OK", CreatedAt: now, UpdatedAt: now}, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNextTask(ctx, enrollment.Runner.ID)
	if err != nil || claimed == nil || claimed.ID != created.ID || claimed.TargetAgentID != cloneID || claimed.TargetAgentSourceID != sourceID || claimed.TargetAgentLocalKey != "scan-codex" || claimed.TargetAgentType != "codex" {
		t.Fatalf("reused claimed task=%+v, err=%v", claimed, err)
	}

	if _, err := repository.HeartbeatRunner(ctx, enrollment.Runner.ID, "0.2.0", "stable-owner-mac", []domain.AgentID{sourceID}, []string{"codex"}, nil); err != nil {
		t.Fatal(err)
	}
	shared, err = repository.ListAgents(ctx, second.ID, owner.ID)
	if err != nil || len(shared) != 1 || shared[0].Status != domain.AgentBusy || shared[0].RunningTaskCount != 1 {
		t.Fatalf("reused Agent status after source heartbeat=%+v, err=%v", shared, err)
	}
	if err := database.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE agents SET status = 'offline' WHERE id = ?`, cloneID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM agents WHERE id = ?`, sourceID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// A source workspace may have been deleted, leaving a Runner with only its
	// old credential map. Its alias must keep the copied Agent online even for
	// an older Client that cannot send the new Agent type metadata.
	if _, err := repository.HeartbeatRunner(ctx, enrollment.Runner.ID, "0.2.0", "stable-owner-mac", []domain.AgentID{sourceID}, nil, nil); err != nil {
		t.Fatal(err)
	}
	shared, err = repository.ListAgents(ctx, second.ID, owner.ID)
	if err != nil || len(shared) != 1 || shared[0].Status != domain.AgentBusy || shared[0].RunningTaskCount != 1 {
		t.Fatalf("reused Agent status after legacy alias heartbeat=%+v, err=%v", shared, err)
	}
}

func TestHeartbeatCatalogAdoptsUniqueLegacyScanIdentity(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	owner := domain.User{ID: "catalog-owner", Username: "owner", DisplayName: "Owner", PasswordHash: "x", Role: domain.UserAdmin, CreatedAt: now, UpdatedAt: now}
	if err := NewAuthRepository(database).CreateUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	repository := NewCollabRepository(database)
	workspace := domain.Workspace{ID: "catalog-ws", Name: "Catalog", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateWorkspace(ctx, workspace, owner.ID); err != nil {
		t.Fatal(err)
	}
	enrollment, err := repository.EnrollRunner(ctx, owner.ID, workspace.ID, "catalog-token", "Owner Mac", "catalog-stable-device", "darwin", "arm64", "old", []domain.AgentRegistration{{
		LocalID: "scan-codex", Type: "codex", DisplayName: "Codex Local", Available: true, Capabilities: []byte(`{"execution":true}`),
	}})
	if err != nil {
		t.Fatal(err)
	}
	legacyID := enrollment.Agents[0].AgentID
	mappings, err := repository.HeartbeatRunner(ctx, enrollment.Runner.ID, "new", "catalog-stable-device", []domain.AgentID{legacyID}, []string{"codex"}, []domain.AgentRegistration{{
		LocalID: "codex-local", Type: "codex", DisplayName: "owner Codex", Available: true,
		Capabilities: []byte(`{"execution":true,"transport":"native","catalog_tier":"native"}`),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(mappings) != 1 || mappings[0].AgentID != legacyID || mappings[0].LocalID != "codex-local" {
		t.Fatalf("catalog mapping = %+v, want inherited Agent %s", mappings, legacyID)
	}
	var count int
	var runtimeLocalKey, sourceDisplayName string
	if err := database.Read(ctx, func(db *sql.DB) error {
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agents WHERE runner_id = ? AND type = 'codex'`, enrollment.Runner.ID).Scan(&count); err != nil {
			return err
		}
		return db.QueryRowContext(ctx, `SELECT runtime_local_key, source_display_name FROM agents WHERE id = ?`, legacyID).Scan(&runtimeLocalKey, &sourceDisplayName)
	}); err != nil {
		t.Fatal(err)
	}
	if count != 1 || runtimeLocalKey != "codex-local" || sourceDisplayName != "Owner Codex" {
		t.Fatalf("catalog identity count=%d runtime_local_key=%q source_display_name=%q", count, runtimeLocalKey, sourceDisplayName)
	}
}

func TestRunnerSupportsScopedCapabilities(t *testing.T) {
	for version, want := range map[string]bool{"dev": true, "development": true, "0.1.9": false, "0.2.0-permissions": true, "v0.3.1": true, "1.0.0": true, "": false} {
		if got := runnerSupportsScopedCapabilities(version); got != want {
			t.Fatalf("runnerSupportsScopedCapabilities(%q) = %v, want %v", version, got, want)
		}
	}
}

func TestRoomOwnerCanAddAnotherMembersSharedDevelopmentAgent(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	owner := domain.User{ID: "user_owner", Username: "owner", DisplayName: "Owner", PasswordHash: "x", Role: domain.UserAdmin, CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "user_member", Username: "member", DisplayName: "Member", PasswordHash: "x", Role: domain.UserMember, CreatedAt: now, UpdatedAt: now}
	authRepository := NewAuthRepository(database)
	for _, user := range []domain.User{owner, member} {
		if err := authRepository.CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}

	repository := NewCollabRepository(database)
	workspace := domain.Workspace{ID: "ws_shared_agent", Name: "Shared Agent", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateWorkspace(ctx, workspace, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)`, workspace.ID, member.ID, formatTime(now))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	room := domain.Room{ID: "room_shared_agent", WorkspaceID: workspace.ID, Name: "Review", CreatedAt: now}
	if err := repository.CreateRoom(ctx, room, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddRoomMember(ctx, room.ID, owner.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	enrollment, err := repository.EnrollRunner(ctx, member.ID, workspace.ID, "member-token", "Member PC", "member-installation", "windows", "amd64", "dev", []domain.AgentRegistration{{
		LocalID: "member-hermes", Type: "hermes", DisplayName: "Hermes", Capabilities: []byte(`{"execution":true}`), Available: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	agentID := enrollment.Agents[0].AgentID
	if err := repository.ImportAgent(ctx, workspace.ID, agentID, member.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateAgentAccessPolicy(ctx, workspace.ID, domain.AgentAccessPolicy{AgentID: agentID, Visibility: domain.AgentVisibilityWorkspace, InvocationScope: domain.AgentInvocationRoomMembers}, member.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE agents SET status = 'offline' WHERE id = ?`, agentID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddRoomAgent(ctx, room.ID, agentID, owner.ID); err != nil {
		t.Fatalf("room owner could not add another member's shared offline Agent: %v", err)
	}
	agents, err := repository.ListRoomAgents(ctx, room.ID, owner.ID)
	if err != nil || len(agents) != 1 {
		t.Fatalf("room Agents after add = %+v, %v", agents, err)
	}
	if agents[0].AgentID != agentID || agents[0].CanManage || !agents[0].CanInvoke {
		t.Fatalf("room owner view of shared development Agent = %+v", agents[0])
	}
}

func TestWorkspaceOwnerManagesMemberRoleAndRemoval(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	owner := domain.User{ID: "owner", Username: "owner", DisplayName: "Owner", PasswordHash: "x", Role: domain.UserAdmin, CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "member", Username: "member", DisplayName: "Member", PasswordHash: "x", Role: domain.UserMember, CreatedAt: now, UpdatedAt: now}
	for _, user := range []domain.User{owner, member} {
		if err := NewAuthRepository(database).CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	repository := NewCollabRepository(database)
	workspace := domain.Workspace{ID: "ws", Name: "Workspace", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateWorkspace(ctx, workspace, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)`, workspace.ID, member.ID, formatTime(now))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateWorkspaceMemberRole(ctx, workspace.ID, member.ID, domain.WorkspaceAdmin, member.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("member promoted self: %v", err)
	}
	if err := repository.UpdateWorkspaceMemberRole(ctx, workspace.ID, member.ID, domain.WorkspaceAdmin, owner.ID); err != nil {
		t.Fatal(err)
	}
	members, err := repository.ListWorkspaceMembers(ctx, workspace.ID, owner.ID)
	if err != nil || len(members) != 2 || members[1].WorkspaceRole != domain.WorkspaceAdmin {
		t.Fatalf("members after promotion = %+v, %v", members, err)
	}
	if err := repository.RemoveWorkspaceMember(ctx, workspace.ID, member.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	members, err = repository.ListWorkspaceMembers(ctx, workspace.ID, owner.ID)
	if err != nil || len(members) != 1 || members[0].ID != owner.ID {
		t.Fatalf("members after removal = %+v, %v", members, err)
	}
}

func TestRoomOwnerAndAdminManageOnlyTheCurrentRoom(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	owner := domain.User{ID: "room-owner", Username: "room-owner", DisplayName: "Room Owner", PasswordHash: "x", Role: domain.UserAdmin, CreatedAt: now, UpdatedAt: now}
	admin := domain.User{ID: "room-admin", Username: "room-admin", DisplayName: "Room Admin", PasswordHash: "x", Role: domain.UserMember, CreatedAt: now, UpdatedAt: now}
	member := domain.User{ID: "room-member", Username: "room-member", DisplayName: "Room Member", PasswordHash: "x", Role: domain.UserMember, CreatedAt: now, UpdatedAt: now}
	for _, user := range []domain.User{owner, admin, member} {
		if err := NewAuthRepository(database).CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	repository := NewCollabRepository(database)
	workspace := domain.Workspace{ID: "room-ws", Name: "Room Roles", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateWorkspace(ctx, workspace, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Write(ctx, func(tx *sql.Tx) error {
		for _, userID := range []domain.UserID{admin.ID, member.ID} {
			if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)`, workspace.ID, userID, formatTime(now)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	room := domain.Room{ID: "room-roles", WorkspaceID: workspace.ID, Name: "Roles", CreatedAt: now}
	if err := repository.CreateRoom(ctx, room, owner.ID); err != nil {
		t.Fatal(err)
	}
	for _, userID := range []domain.UserID{admin.ID, member.ID} {
		if err := repository.AddRoomMember(ctx, room.ID, owner.ID, userID); err != nil {
			t.Fatal(err)
		}
	}
	if err := repository.UpdateRoomMemberRole(ctx, room.ID, admin.ID, domain.RoomAdmin, admin.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("member promoted self: %v", err)
	}
	if err := repository.UpdateRoomMemberRole(ctx, room.ID, admin.ID, domain.RoomAdmin, owner.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.NewService(NewAuthRepository(database)).CreateInvite(ctx, admin.ID, workspace.ID, domain.WorkspaceMember, time.Hour, room.ID); err != nil {
		t.Fatalf("room admin could not invite a regular member: %v", err)
	}
	if err := repository.UpdateRoomMemberRole(ctx, room.ID, member.ID, domain.RoomAdmin, admin.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("room admin promoted another member: %v", err)
	}
	if err := repository.RemoveRoomMember(ctx, room.ID, owner.ID, admin.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("room admin removed owner: %v", err)
	}
	if err := repository.RemoveRoomMember(ctx, room.ID, member.ID, admin.ID); err != nil {
		t.Fatal(err)
	}
	members, err := repository.ListRoomMembers(ctx, room.ID, owner.ID)
	if err != nil || len(members) != 2 || members[0].RoomRole != domain.RoomOwner || members[1].RoomRole != domain.RoomAdmin {
		t.Fatalf("room members after moderation = %+v, %v", members, err)
	}
	workspaceMembers, err := repository.ListWorkspaceMembers(ctx, workspace.ID, owner.ID)
	if err != nil || len(workspaceMembers) != 3 {
		t.Fatalf("room removal changed workspace membership: %+v, %v", workspaceMembers, err)
	}
}
