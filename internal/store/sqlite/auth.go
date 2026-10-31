package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
)

type AuthRepository struct {
	store *Store
}

func NewAuthRepository(store *Store) *AuthRepository {
	return &AuthRepository{store: store}
}

func (r *AuthRepository) CreateUser(ctx context.Context, user domain.User) error {
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO users (id, username, display_name, password_hash, role, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, user.ID, user.Username, user.DisplayName, user.PasswordHash, user.Role, formatTime(user.CreatedAt), formatTime(user.UpdatedAt))
		return err
	})
	if isUniqueConstraint(err) {
		return auth.ErrConflict
	}
	return err
}

func (r *AuthRepository) UserByUsername(ctx context.Context, username string) (domain.User, error) {
	var user domain.User
	err := r.store.Read(ctx, func(db *sql.DB) error {
		return scanUser(db.QueryRowContext(ctx, `SELECT id, username, display_name, avatar_url, bio, current_focus, password_hash, role, created_at, updated_at FROM users WHERE username = ? AND archived_at IS NULL`, username), &user)
	})
	return user, mapNotFound(err)
}

func (r *AuthRepository) UserByID(ctx context.Context, id domain.UserID) (domain.User, error) {
	var user domain.User
	err := r.store.Read(ctx, func(db *sql.DB) error {
		return scanUser(db.QueryRowContext(ctx, `SELECT id, username, display_name, avatar_url, bio, current_focus, password_hash, role, created_at, updated_at FROM users WHERE id = ?`, id), &user)
	})
	return user, mapNotFound(err)
}

func (r *AuthRepository) ListUsers(ctx context.Context) ([]domain.User, error) {
	users := make([]domain.User, 0)
	err := r.store.Read(ctx, func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `
SELECT id, username, display_name, avatar_url, bio, current_focus, password_hash, role, created_at, updated_at
FROM users WHERE archived_at IS NULL
ORDER BY CASE role WHEN 'admin' THEN 0 ELSE 1 END, display_name, id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var user domain.User
			if err := scanUser(rows, &user); err != nil {
				return err
			}
			users = append(users, user)
		}
		return rows.Err()
	})
	return users, err
}

func (r *AuthRepository) CreateSession(ctx context.Context, session auth.Session) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at)
VALUES (?, ?, ?, ?, ?)`, session.ID, session.UserID, session.TokenHash, formatTime(session.ExpiresAt), formatTime(session.CreatedAt))
		return err
	})
}

func (r *AuthRepository) UserBySessionHash(ctx context.Context, tokenHash string, now time.Time) (domain.User, error) {
	var user domain.User
	err := r.store.Read(ctx, func(db *sql.DB) error {
		return scanUser(db.QueryRowContext(ctx, `
SELECT u.id, u.username, u.display_name, u.avatar_url, u.bio, u.current_focus, u.password_hash, u.role, u.created_at, u.updated_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = ? AND s.revoked_at IS NULL AND s.expires_at > ? AND u.archived_at IS NULL`, tokenHash, formatTime(now)), &user)
	})
	return user, mapNotFound(err)
}

func (r *AuthRepository) UpdateUserProfile(ctx context.Context, userID domain.UserID, input auth.ProfileInput, now time.Time) (domain.User, error) {
	var user domain.User
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
UPDATE users
SET display_name = ?, avatar_url = ?, bio = ?, current_focus = ?, updated_at = ?
WHERE id = ?`, input.DisplayName, input.AvatarURL, input.Bio, input.CurrentFocus, formatTime(now), userID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		if err := refreshManagedAgentDisplayNamesTx(ctx, tx, userID, input.DisplayName); err != nil {
			return err
		}
		return scanUser(tx.QueryRowContext(ctx, `SELECT id, username, display_name, avatar_url, bio, current_focus, password_hash, role, created_at, updated_at FROM users WHERE id = ?`, userID), &user)
	})
	return user, mapNotFound(err)
}

func (r *AuthRepository) UpdateUserRole(ctx context.Context, userID domain.UserID, role domain.UserRole, now time.Time) (domain.User, error) {
	var user domain.User
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		var currentRole domain.UserRole
		if err := tx.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ? AND archived_at IS NULL`, userID).Scan(&currentRole); err != nil {
			return mapNotFound(err)
		}
		if currentRole == domain.UserAdmin && role == domain.UserMember {
			var admins int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND archived_at IS NULL`).Scan(&admins); err != nil {
				return err
			}
			if admins <= 1 {
				return auth.ErrConflict
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users SET role = ?, updated_at = ? WHERE id = ? AND archived_at IS NULL`, role, formatTime(now), userID); err != nil {
			return err
		}
		return scanUser(tx.QueryRowContext(ctx, `SELECT id, username, display_name, avatar_url, bio, current_focus, password_hash, role, created_at, updated_at FROM users WHERE id = ?`, userID), &user)
	})
	return user, mapNotFound(err)
}

func (r *AuthRepository) ArchiveUser(ctx context.Context, userID domain.UserID, now time.Time) error {
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		var role domain.UserRole
		if err := tx.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ? AND archived_at IS NULL`, userID).Scan(&role); err != nil {
			return mapNotFound(err)
		}
		if role == domain.UserAdmin {
			var admins int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND archived_at IS NULL`).Scan(&admins); err != nil {
				return err
			}
			if admins <= 1 {
				return auth.ErrLastHubAdmin
			}
		}
		var workspaces int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspaces WHERE owner_user_id = ?`, userID).Scan(&workspaces); err != nil {
			return err
		}
		if workspaces > 0 {
			return auth.ErrAccountOwnsWorkspace
		}
		var infrastructure int
		if err := tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM agents WHERE owner_user_id = ?) + (SELECT COUNT(*) FROM devices WHERE owner_user_id = ?)`, userID, userID).Scan(&infrastructure); err != nil {
			return err
		}
		if infrastructure > 0 {
			return auth.ErrAccountHasInfrastructure
		}
		// Membership and invitations are active access-control records, not
		// historic conversation data.  Remove/revoke them while preserving the
		// user row so old messages can still show their original sender.
		if _, err := tx.ExecContext(ctx, `DELETE FROM room_members WHERE user_id = ?`, userID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_members WHERE user_id = ?`, userID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, formatTime(now), userID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE invites SET revoked_at = ? WHERE created_by_user_id = ? AND revoked_at IS NULL`, formatTime(now), userID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE runner_invites SET revoked_at = ? WHERE created_by_user_id = ? AND revoked_at IS NULL`, formatTime(now), userID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE users SET archived_at = ?, updated_at = ? WHERE id = ? AND archived_at IS NULL`, formatTime(now), formatTime(now), userID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		return nil
	})
	return mapNotFound(err)
}

func (r *AuthRepository) RevokeSession(ctx context.Context, tokenHash string, now time.Time) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE token_hash = ? AND revoked_at IS NULL`, formatTime(now), tokenHash)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return auth.ErrNotFound
		}
		return nil
	})
}

func (r *AuthRepository) WorkspaceRole(ctx context.Context, workspaceID domain.WorkspaceID, userID domain.UserID) (domain.WorkspaceRole, error) {
	var role domain.WorkspaceRole
	err := r.store.Read(ctx, func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, userID).Scan(&role)
	})
	return role, mapNotFound(err)
}

func (r *AuthRepository) RoomRole(ctx context.Context, roomID domain.RoomID, userID domain.UserID) (domain.RoomRole, error) {
	var role domain.RoomRole
	err := r.store.Read(ctx, func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `SELECT role FROM room_members WHERE room_id = ? AND user_id = ?`, roomID, userID).Scan(&role)
	})
	return role, mapNotFound(err)
}

func (r *AuthRepository) CreateInvite(ctx context.Context, invite auth.Invite) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		if invite.RoomID != "" {
			var roomWorkspaceID domain.WorkspaceID
			if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM rooms WHERE id = ?`, invite.RoomID).Scan(&roomWorkspaceID); err != nil {
				return mapNotFound(err)
			}
			if roomWorkspaceID != invite.WorkspaceID {
				return auth.ErrInvalidInput
			}
		}
		var roomID any
		if invite.RoomID != "" {
			roomID = invite.RoomID
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO invites (id, workspace_id, room_id, created_by_user_id, code_hash, role, expires_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, invite.ID, invite.WorkspaceID, roomID, invite.CreatedByUserID, invite.CodeHash, invite.Role, formatTime(invite.ExpiresAt), formatTime(invite.CreatedAt))
		return err
	})
}

func (r *AuthRepository) ValidateInvite(ctx context.Context, codeHash string, now time.Time) error {
	err := r.store.Read(ctx, func(db *sql.DB) error {
		var marker int
		return db.QueryRowContext(ctx, `
SELECT 1 FROM invites
WHERE code_hash = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > ?`, codeHash, formatTime(now)).Scan(&marker)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return auth.ErrInviteInvalid
	}
	return err
}

func (r *AuthRepository) CreateRunnerInvite(ctx context.Context, invite auth.RunnerInvite) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO runner_invites (id, workspace_id, owner_user_id, created_by_user_id, code_hash, expires_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, invite.ID, invite.WorkspaceID, invite.OwnerUserID, invite.CreatedByUserID,
			invite.CodeHash, formatTime(invite.ExpiresAt), formatTime(invite.CreatedAt))
		return err
	})
}

func (r *AuthRepository) ConsumeInvite(ctx context.Context, codeHash string, userID domain.UserID, now time.Time) (domain.WorkspaceMembership, error) {
	var membership domain.WorkspaceMembership
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		var workspaceID domain.WorkspaceID
		var role domain.WorkspaceRole
		var roomID sql.NullString
		err := tx.QueryRowContext(ctx, `
SELECT workspace_id, role, room_id FROM invites
WHERE code_hash = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > ?`, codeHash, formatTime(now)).Scan(&workspaceID, &role, &roomID)
		if errors.Is(err, sql.ErrNoRows) {
			return auth.ErrInviteInvalid
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO workspace_members (workspace_id, user_id, role, joined_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(workspace_id, user_id) DO NOTHING`, workspaceID, userID, role, formatTime(now)); err != nil {
			return err
		}
		if roomID.Valid && roomID.String != "" {
			var roomWorkspaceID domain.WorkspaceID
			if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM rooms WHERE id = ?`, roomID.String).Scan(&roomWorkspaceID); err != nil {
				return mapNotFound(err)
			}
			if roomWorkspaceID != workspaceID {
				return auth.ErrInviteInvalid
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO room_members (room_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?) ON CONFLICT(room_id, user_id) DO NOTHING`, roomID.String, userID, formatTime(now)); err != nil {
				return err
			}
		}
		result, err := tx.ExecContext(ctx, `
UPDATE invites SET used_by_user_id = ?, used_at = ?
WHERE code_hash = ? AND used_at IS NULL AND revoked_at IS NULL`, userID, formatTime(now), codeHash)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil || rows != 1 {
			return auth.ErrInviteInvalid
		}
		var actualRole domain.WorkspaceRole
		var joinedAt string
		if err := tx.QueryRowContext(ctx, `SELECT role, joined_at FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, userID).Scan(&actualRole, &joinedAt); err != nil {
			return err
		}
		joined, err := time.Parse(time.RFC3339Nano, joinedAt)
		if err != nil {
			return fmt.Errorf("parse membership joined_at: %w", err)
		}
		membership = domain.WorkspaceMembership{WorkspaceID: workspaceID, UserID: userID, Role: actualRole, JoinedAt: joined}
		return nil
	})
	return membership, err
}

type rowScanner interface {
	Scan(...any) error
}

func scanUser(row rowScanner, user *domain.User) error {
	var createdAt, updatedAt string
	if err := row.Scan(&user.ID, &user.Username, &user.DisplayName, &user.AvatarURL, &user.Bio, &user.CurrentFocus, &user.PasswordHash, &user.Role, &createdAt, &updatedAt); err != nil {
		return err
	}
	var err error
	if user.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return fmt.Errorf("parse user created_at: %w", err)
	}
	if user.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return fmt.Errorf("parse user updated_at: %w", err)
	}
	return nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func mapNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return auth.ErrNotFound
	}
	return err
}

func isUniqueConstraint(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
