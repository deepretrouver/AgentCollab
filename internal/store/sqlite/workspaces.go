package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
)

type CollabRepository struct {
	store *Store
}

func NewCollabRepository(store *Store) *CollabRepository {
	return &CollabRepository{store: store}
}

func (r *CollabRepository) CreateWorkspace(ctx context.Context, workspace domain.Workspace, ownerID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO workspaces (id, name, owner_user_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)`, workspace.ID, workspace.Name, ownerID, formatTime(workspace.CreatedAt), formatTime(workspace.UpdatedAt)); err != nil {
			if isUniqueConstraint(err) {
				return auth.ErrConflict
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO workspace_members (workspace_id, user_id, role, joined_at)
VALUES (?, ?, 'owner', ?)`, workspace.ID, ownerID, formatTime(workspace.CreatedAt)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO event_sequence (workspace_id, next_sequence) VALUES (?, 1)`, workspace.ID)
		return err
	})
}

func (r *CollabRepository) ListWorkspaces(ctx context.Context, userID domain.UserID) ([]domain.Workspace, error) {
	workspaces := make([]domain.Workspace, 0)
	err := r.store.Read(ctx, func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `
SELECT w.id, w.name, w.owner_user_id, wm.role, w.created_at, w.updated_at
FROM workspaces w
JOIN workspace_members wm ON wm.workspace_id = w.id
WHERE wm.user_id = ?
ORDER BY w.created_at, w.id`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			workspace, err := scanWorkspaceWithRole(rows)
			if err != nil {
				return err
			}
			workspaces = append(workspaces, workspace)
		}
		return rows.Err()
	})
	return workspaces, err
}

func (r *CollabRepository) UpdateWorkspace(ctx context.Context, workspaceID domain.WorkspaceID, name string, actorID domain.UserID) (domain.Workspace, error) {
	var workspace domain.Workspace
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		role, err := workspaceRoleTx(ctx, tx, workspaceID, actorID)
		if err != nil {
			return err
		}
		if role != domain.WorkspaceOwner {
			return auth.ErrForbidden
		}
		now := time.Now().UTC()
		result, err := tx.ExecContext(ctx, `UPDATE workspaces SET name = ?, updated_at = ? WHERE id = ?`, name, formatTime(now), workspaceID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		var createdAt, updatedAt string
		if err := tx.QueryRowContext(ctx, `SELECT id, name, owner_user_id, created_at, updated_at FROM workspaces WHERE id = ?`, workspaceID).Scan(&workspace.ID, &workspace.Name, &workspace.OwnerUserID, &createdAt, &updatedAt); err != nil {
			return mapNotFound(err)
		}
		workspace.Role = role
		workspace.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return err
		}
		workspace.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		return err
	})
	return workspace, err
}

func (r *CollabRepository) DeleteWorkspace(ctx context.Context, workspaceID domain.WorkspaceID, actorID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		role, err := workspaceRoleTx(ctx, tx, workspaceID, actorID)
		if err != nil {
			return err
		}
		if role != domain.WorkspaceOwner {
			return auth.ErrForbidden
		}
		var activeTasks int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE workspace_id = ? AND state IN ('queued', 'dispatched', 'running')`, workspaceID).Scan(&activeTasks); err != nil {
			return err
		}
		if activeTasks > 0 {
			return auth.ErrWorkspaceHasActiveTasks
		}
		// A Runner represents a real computer, whereas a workspace is simply a
		// collaboration boundary.  An Agent may have been imported into another
		// workspace, so re-home that device before the source workspace cascades.
		// This preserves the Runner token and all remaining Agent records without
		// asking the owner to manually re-pair a computer just to delete a group.
		if err := rehomeSharedRunnerDevicesTx(ctx, tx, workspaceID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM workspaces WHERE id = ?`, workspaceID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		return nil
	})
}

// rehomeSharedRunnerDevicesTx moves a physical device to the earliest
// workspace that still contains one of its Agents.  The Agent record in the
// deleted workspace is allowed to cascade; copies in other workspaces retain
// their Runner and can keep accepting tasks with the same local credentials.
func rehomeSharedRunnerDevicesTx(ctx context.Context, tx *sql.Tx, deletingWorkspaceID domain.WorkspaceID) error {
	// A Client persists its original Agent IDs locally. Before the source Agent
	// rows cascade with the deleted workspace, preserve aliases for any copies
	// that survive elsewhere. This keeps old Client versions online too.
	if _, err := tx.ExecContext(ctx, `
INSERT INTO runner_agent_aliases (runner_id, legacy_agent_id, runtime_local_key)
SELECT source.runner_id, source.id, COALESCE(NULLIF(source.runtime_local_key, ''), source.local_key)
FROM agents source
WHERE source.workspace_id = ?
  AND EXISTS (
      SELECT 1
      FROM agents copy
      WHERE copy.runner_id = source.runner_id
        AND copy.workspace_id != source.workspace_id
        AND COALESCE(NULLIF(copy.runtime_local_key, ''), copy.local_key) = COALESCE(NULLIF(source.runtime_local_key, ''), source.local_key)
  )
ON CONFLICT(runner_id, legacy_agent_id) DO UPDATE SET runtime_local_key = excluded.runtime_local_key`, deletingWorkspaceID); err != nil {
		return err
	}

	type rehome struct {
		deviceID    domain.DeviceID
		workspaceID domain.WorkspaceID
	}

	rows, err := tx.QueryContext(ctx, `
SELECT d.id,
       (
           SELECT a.workspace_id
           FROM agents a
           WHERE a.runner_id = r.id AND a.workspace_id != ?
           ORDER BY a.created_at, a.id
           LIMIT 1
       )
FROM devices d
JOIN runners r ON r.device_id = d.id
WHERE d.workspace_id = ?
  AND EXISTS (
      SELECT 1
      FROM agents a
      WHERE a.runner_id = r.id AND a.workspace_id != ?
  )`, deletingWorkspaceID, deletingWorkspaceID, deletingWorkspaceID)
	if err != nil {
		return err
	}
	defer rows.Close()

	changes := make([]rehome, 0)
	for rows.Next() {
		var change rehome
		if err := rows.Scan(&change.deviceID, &change.workspaceID); err != nil {
			return err
		}
		changes = append(changes, change)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, change := range changes {
		if _, err := tx.ExecContext(ctx, `UPDATE devices SET workspace_id = ? WHERE id = ?`, change.workspaceID, change.deviceID); err != nil {
			return err
		}
	}
	return nil
}

func scanWorkspace(row rowScanner) (domain.Workspace, error) {
	var workspace domain.Workspace
	var createdAt, updatedAt string
	if err := row.Scan(&workspace.ID, &workspace.Name, &workspace.OwnerUserID, &createdAt, &updatedAt); err != nil {
		return domain.Workspace{}, err
	}
	var err error
	workspace.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("parse workspace created_at: %w", err)
	}
	workspace.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("parse workspace updated_at: %w", err)
	}
	return workspace, nil
}

func scanWorkspaceWithRole(row rowScanner) (domain.Workspace, error) {
	var workspace domain.Workspace
	var createdAt, updatedAt string
	if err := row.Scan(&workspace.ID, &workspace.Name, &workspace.OwnerUserID, &workspace.Role, &createdAt, &updatedAt); err != nil {
		return domain.Workspace{}, err
	}
	var err error
	workspace.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.Workspace{}, err
	}
	workspace.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	return workspace, err
}

func (r *CollabRepository) ListWorkspaceMembers(ctx context.Context, workspaceID domain.WorkspaceID, requesterID domain.UserID) ([]domain.User, error) {
	members := make([]domain.User, 0)
	err := r.store.Read(ctx, func(db *sql.DB) error {
		var requesterRole domain.WorkspaceRole
		if err := db.QueryRowContext(ctx, `SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, requesterID).Scan(&requesterRole); err != nil {
			return mapNotFound(err)
		}
		rows, err := db.QueryContext(ctx, `
SELECT u.id, u.username, u.display_name, u.avatar_url, u.bio, u.current_focus, u.password_hash, u.role, u.created_at, u.updated_at, wm.role
FROM workspace_members wm JOIN users u ON u.id = wm.user_id
WHERE wm.workspace_id = ? ORDER BY CASE wm.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, u.display_name, u.id`, workspaceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var user domain.User
			var createdAt, updatedAt string
			if err := rows.Scan(&user.ID, &user.Username, &user.DisplayName, &user.AvatarURL, &user.Bio, &user.CurrentFocus, &user.PasswordHash, &user.Role, &createdAt, &updatedAt, &user.WorkspaceRole); err != nil {
				return err
			}
			user.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
			if err != nil {
				return err
			}
			user.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
			if err != nil {
				return err
			}
			members = append(members, user)
		}
		return rows.Err()
	})
	return members, err
}

func (r *CollabRepository) UpdateWorkspaceMemberRole(ctx context.Context, workspaceID domain.WorkspaceID, targetID domain.UserID, role domain.WorkspaceRole, actorID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		actorRole, err := workspaceRoleTx(ctx, tx, workspaceID, actorID)
		if err != nil {
			return err
		}
		if actorRole != domain.WorkspaceOwner || targetID == actorID || (role != domain.WorkspaceAdmin && role != domain.WorkspaceMember) {
			return auth.ErrForbidden
		}
		result, err := tx.ExecContext(ctx, `UPDATE workspace_members SET role = ? WHERE workspace_id = ? AND user_id = ? AND role != 'owner'`, role, workspaceID, targetID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		return nil
	})
}

func (r *CollabRepository) RemoveWorkspaceMember(ctx context.Context, workspaceID domain.WorkspaceID, targetID, actorID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		actorRole, err := workspaceRoleTx(ctx, tx, workspaceID, actorID)
		if err != nil {
			return err
		}
		if actorRole != domain.WorkspaceOwner || targetID == actorID {
			return auth.ErrForbidden
		}
		var targetRole domain.WorkspaceRole
		if err := tx.QueryRowContext(ctx, `SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, targetID).Scan(&targetRole); err != nil {
			return mapNotFound(err)
		}
		if targetRole == domain.WorkspaceOwner {
			return auth.ErrForbidden
		}
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE workspace_id = ? AND target_agent_id IN (SELECT id FROM agents WHERE owner_user_id = ?) AND state IN ('queued','dispatched','running')`, workspaceID, targetID).Scan(&active); err != nil {
			return err
		}
		if active > 0 {
			return auth.ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_sessions WHERE agent_id IN (SELECT id FROM agents WHERE workspace_id = ? AND owner_user_id = ?)`, workspaceID, targetID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM room_agent_settings WHERE agent_id IN (SELECT id FROM agents WHERE workspace_id = ? AND owner_user_id = ?)`, workspaceID, targetID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM room_agents WHERE agent_id IN (SELECT id FROM agents WHERE workspace_id = ? AND owner_user_id = ?)`, workspaceID, targetID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE agent_access_policies SET archived = 1, updated_by_user_id = ?, updated_at = ? WHERE agent_id IN (SELECT id FROM agents WHERE workspace_id = ? AND owner_user_id = ?)`, actorID, formatTime(time.Now().UTC()), workspaceID, targetID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE agents SET imported = 0, status = 'unavailable', updated_at = ? WHERE workspace_id = ? AND owner_user_id = ?`, formatTime(time.Now().UTC()), workspaceID, targetID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE runners SET status = 'revoked' WHERE device_id IN (SELECT id FROM devices WHERE workspace_id = ? AND owner_user_id = ?)`, workspaceID, targetID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE devices SET status = 'revoked' WHERE workspace_id = ? AND owner_user_id = ?`, workspaceID, targetID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM room_members WHERE user_id = ? AND room_id IN (SELECT id FROM rooms WHERE workspace_id = ?)`, targetID, workspaceID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, targetID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		return nil
	})
}

func workspaceRoleTx(ctx context.Context, tx *sql.Tx, workspaceID domain.WorkspaceID, userID domain.UserID) (domain.WorkspaceRole, error) {
	var role domain.WorkspaceRole
	err := tx.QueryRowContext(ctx, `SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", auth.ErrNotFound
	}
	return role, err
}
