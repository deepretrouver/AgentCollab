package auth_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
	store "agentcollab.local/agentcollab/internal/store/sqlite"
)

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := auth.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.VerifyPassword(hash, "correct horse battery staple"); err != nil {
		t.Fatalf("VerifyPassword(correct) error = %v", err)
	}
	if err := auth.VerifyPassword(hash, "wrong password"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("VerifyPassword(wrong) error = %v", err)
	}
	if err := auth.VerifyPassword("not-a-hash", "password"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("VerifyPassword(malformed) error = %v", err)
	}
}

func TestPasswordAllowsAnyNonEmptyValue(t *testing.T) {
	hash, err := auth.HashPassword("1")
	if err != nil {
		t.Fatalf("HashPassword(short non-empty) error = %v", err)
	}
	if err := auth.VerifyPassword(hash, "1"); err != nil {
		t.Fatalf("VerifyPassword(short non-empty) error = %v", err)
	}
	if _, err := auth.HashPassword(""); err == nil {
		t.Fatal("HashPassword(empty) error = nil, want error")
	}
}

func TestRegisterAllowsChineseUsername(t *testing.T) {
	ctx := context.Background()
	service, database := newService(t)
	defer database.Close()

	registered, err := service.Register(ctx, "杨凯翔", "杨凯翔", "1")
	if err != nil {
		t.Fatalf("Register(chinese username) error = %v", err)
	}
	if registered.User.Username != "杨凯翔" {
		t.Fatalf("registered username = %q, want Chinese username", registered.User.Username)
	}
	if _, err := service.Login(ctx, "杨凯翔", "1"); err != nil {
		t.Fatalf("Login(chinese username) error = %v", err)
	}
}

func TestLoginAuthenticateAndLogout(t *testing.T) {
	ctx := context.Background()
	service, database := newService(t)
	defer database.Close()
	user, err := service.CreateUser(ctx, "owner", "Owner", "password-1234", domain.UserAdmin)
	if err != nil {
		t.Fatal(err)
	}

	login, err := service.Login(ctx, "owner", "password-1234")
	if err != nil {
		t.Fatal(err)
	}
	if login.Token == "" || login.User.ID != user.ID || !login.ExpiresAt.After(time.Now()) {
		t.Fatalf("Login() = %#v", login)
	}
	authenticated, err := service.Authenticate(ctx, login.Token)
	if err != nil || authenticated.ID != user.ID {
		t.Fatalf("Authenticate() = %#v, %v", authenticated, err)
	}
	if err := service.Logout(ctx, login.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, login.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("Authenticate(after logout) error = %v", err)
	}
}

func TestRegisterCreatesStandaloneMemberAndProfileIsMemberOwned(t *testing.T) {
	ctx := context.Background()
	service, database := newService(t)
	defer database.Close()

	registered, err := service.Register(ctx, "new-user", "新成员", "password-1234")
	if err != nil {
		t.Fatal(err)
	}
	if registered.User.Role != domain.UserMember || registered.Token == "" {
		t.Fatalf("Register() = %#v", registered)
	}
	updated, err := service.UpdateProfile(ctx, registered.User.ID, auth.ProfileInput{
		DisplayName:  "新成员（研究）",
		Bio:          "负责投资研究。",
		CurrentFocus: "验证局域网协作",
		AvatarURL:    "data:image/png;base64,aGVsbG8=",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "新成员（研究）" || updated.Bio != "负责投资研究。" || updated.CurrentFocus != "验证局域网协作" || updated.AvatarURL == "" {
		t.Fatalf("UpdateProfile() = %#v", updated)
	}
	var memberships int
	if err := database.Read(ctx, func(db *sql.DB) error {
		return db.QueryRow(`SELECT COUNT(*) FROM workspace_members WHERE user_id = ?`, registered.User.ID).Scan(&memberships)
	}); err != nil {
		t.Fatal(err)
	}
	if memberships != 0 {
		t.Fatalf("standalone registered user memberships = %d, want 0", memberships)
	}
}

func TestHubAdminCanPromoteExistingAccountWithoutChangingWorkspaceMembership(t *testing.T) {
	ctx := context.Background()
	service, database := newService(t)
	defer database.Close()
	owner, err := service.CreateUser(ctx, "owner", "Owner", "password-1234", domain.UserAdmin)
	if err != nil {
		t.Fatal(err)
	}
	member, err := service.CreateUser(ctx, "member", "Member", "password-5678", domain.UserMember)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListHubUsers(ctx, member.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("member ListHubUsers() error = %v", err)
	}
	updated, err := service.UpdateHubUserRole(ctx, owner.ID, member.ID, domain.UserAdmin)
	if err != nil || updated.Role != domain.UserAdmin {
		t.Fatalf("UpdateHubUserRole() = %#v, %v", updated, err)
	}
	if _, err := service.UpdateHubUserRole(ctx, member.ID, owner.ID, domain.UserMember); !errors.Is(err, auth.ErrDefaultHubAdmin) {
		t.Fatalf("demote default owner error = %v", err)
	}
	if err := service.ArchiveHubUser(ctx, member.ID, owner.ID); !errors.Is(err, auth.ErrDefaultHubAdmin) {
		t.Fatalf("archive default owner error = %v", err)
	}
	if _, err := service.UpdateHubUserRole(ctx, owner.ID, owner.ID, domain.UserMember); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("self demotion error = %v", err)
	}
	var memberships int
	if err := database.Read(ctx, func(db *sql.DB) error {
		return db.QueryRow(`SELECT COUNT(*) FROM workspace_members WHERE user_id = ?`, member.ID).Scan(&memberships)
	}); err != nil {
		t.Fatal(err)
	}
	if memberships != 0 {
		t.Fatalf("Hub admin promotion created %d memberships", memberships)
	}
}

func TestWorkspaceInviteIsAuthorizedAndOneTime(t *testing.T) {
	ctx := context.Background()
	service, database := newService(t)
	defer database.Close()
	owner, err := service.CreateUser(ctx, "owner", "Owner", "password-1234", domain.UserAdmin)
	if err != nil {
		t.Fatal(err)
	}
	member, err := service.CreateUser(ctx, "member", "Member", "password-5678", domain.UserMember)
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := service.CreateUser(ctx, "outsider", "Outsider", "password-9012", domain.UserMember)
	if err != nil {
		t.Fatal(err)
	}
	if err := seedWorkspace(database, owner.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := service.CreateInvite(ctx, outsider.ID, "workspace-1", domain.WorkspaceMember, time.Hour); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("CreateInvite(outsider) error = %v", err)
	}
	invite, err := service.CreateInvite(ctx, owner.ID, "workspace-1", domain.WorkspaceMember, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if invite.Code == "" {
		t.Fatal("CreateInvite() returned empty code")
	}
	membership, err := service.AcceptInvite(ctx, invite.Code, member.ID)
	if err != nil || membership.WorkspaceID != "workspace-1" || membership.Role != domain.WorkspaceMember {
		t.Fatalf("AcceptInvite() = %#v, %v", membership, err)
	}
	if _, err := service.AcceptInvite(ctx, invite.Code, outsider.ID); !errors.Is(err, auth.ErrInviteInvalid) {
		t.Fatalf("AcceptInvite(second use) error = %v", err)
	}
}

func TestAcceptInviteDoesNotDowngradeExistingOwner(t *testing.T) {
	ctx := context.Background()
	service, database := newService(t)
	defer database.Close()
	owner, err := service.CreateUser(ctx, "owner", "Owner", "password-1234", domain.UserAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := seedWorkspace(database, owner.ID); err != nil {
		t.Fatal(err)
	}
	invite, err := service.CreateInvite(ctx, owner.ID, "workspace-1", domain.WorkspaceMember, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	membership, err := service.AcceptInvite(ctx, invite.Code, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if membership.Role != domain.WorkspaceOwner {
		t.Fatalf("membership role = %q, want owner", membership.Role)
	}
}

func TestRegisterWithRoomInviteCreatesHumanMemberAndSession(t *testing.T) {
	ctx := context.Background()
	service, database := newService(t)
	defer database.Close()
	owner, err := service.CreateUser(ctx, "owner", "Owner", "password-1234", domain.UserAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := seedWorkspace(database, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO rooms (id, workspace_id, name) VALUES ('room-1', 'workspace-1', 'Shared room')`); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO room_members (room_id, user_id) VALUES ('room-1', ?)`, owner.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := service.RegisterWithInvite(ctx, "invalid-code", "never-created", "Never Created", "password-5678"); !errors.Is(err, auth.ErrInviteInvalid) {
		t.Fatalf("RegisterWithInvite(invalid) error = %v", err)
	}
	if _, err := service.Login(ctx, "never-created", "password-5678"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("invalid invite created an orphan user: Login error = %v", err)
	}

	invite, err := service.CreateInvite(ctx, owner.ID, "workspace-1", domain.WorkspaceMember, time.Hour, "room-1")
	if err != nil {
		t.Fatal(err)
	}
	login, membership, err := service.RegisterWithInvite(ctx, invite.Code, "new-person", "New Person", "password-5678")
	if err != nil {
		t.Fatal(err)
	}
	if login.Token == "" || login.User.Username != "new-person" || login.User.Role != domain.UserMember {
		t.Fatalf("RegisterWithInvite login = %#v", login)
	}
	if membership.WorkspaceID != "workspace-1" || membership.Role != domain.WorkspaceMember {
		t.Fatalf("RegisterWithInvite membership = %#v", membership)
	}
	var roomMemberships int
	if err := database.Read(ctx, func(db *sql.DB) error {
		return db.QueryRow(`SELECT COUNT(*) FROM room_members WHERE room_id = 'room-1' AND user_id = ?`, login.User.ID).Scan(&roomMemberships)
	}); err != nil {
		t.Fatal(err)
	}
	if roomMemberships != 1 {
		t.Fatalf("room membership count = %d, want 1", roomMemberships)
	}
	if _, _, err := service.RegisterWithInvite(ctx, invite.Code, "another-person", "Another Person", "password-9012"); !errors.Is(err, auth.ErrInviteInvalid) {
		t.Fatalf("RegisterWithInvite(second use) error = %v", err)
	}
}

func newService(t *testing.T) (*auth.Service, *store.Store) {
	t.Helper()
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	return auth.NewService(store.NewAuthRepository(database)), database
}

func seedWorkspace(database *store.Store, ownerID domain.UserID) error {
	return database.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO workspaces (id, name, owner_user_id) VALUES ('workspace-1', 'Workspace', ?)`, ownerID); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ('workspace-1', ?, 'owner')`, ownerID)
		return err
	})
}
