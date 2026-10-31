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

func (r *CollabRepository) CreateRoom(ctx context.Context, room domain.Room, actorID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		role, err := workspaceRoleTx(ctx, tx, room.WorkspaceID, actorID)
		if err != nil {
			return err
		}
		if role != domain.WorkspaceOwner && role != domain.WorkspaceAdmin {
			return auth.ErrForbidden
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO rooms (id, workspace_id, name, created_at) VALUES (?, ?, ?, ?)`, room.ID, room.WorkspaceID, room.Name, formatTime(room.CreatedAt)); err != nil {
			if isUniqueConstraint(err) {
				return auth.ErrConflict
			}
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO room_members (room_id, user_id, role, joined_at) VALUES (?, ?, 'owner', ?)`, room.ID, actorID, formatTime(room.CreatedAt))
		return err
	})
}

func (r *CollabRepository) ListRooms(ctx context.Context, workspaceID domain.WorkspaceID, userID domain.UserID) ([]domain.Room, error) {
	rooms := make([]domain.Room, 0)
	err := r.store.Read(ctx, func(db *sql.DB) error {
		var member int
		if err := db.QueryRowContext(ctx, `SELECT 1 FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, userID).Scan(&member); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return auth.ErrNotFound
			}
			return err
		}
		rows, err := db.QueryContext(ctx, `
SELECT r.id, r.workspace_id, r.name, r.plan_step_id, r.created_at, COALESCE(rp.pinned, 0), rp.pinned_at
FROM rooms r
JOIN room_members rm ON rm.room_id = r.id
LEFT JOIN room_preferences rp ON rp.room_id = r.id AND rp.user_id = ?
WHERE r.workspace_id = ? AND rm.user_id = ?
ORDER BY r.created_at, r.id`, userID, workspaceID, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			room, err := scanRoom(rows)
			if err != nil {
				return err
			}
			rooms = append(rooms, room)
		}
		return rows.Err()
	})
	return rooms, err
}

func (r *CollabRepository) SetRoomPinned(ctx context.Context, roomID domain.RoomID, userID domain.UserID, pinned bool) (domain.Room, error) {
	var room domain.Room
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		var createdAt string
		var planStepID sql.NullString
		if err := tx.QueryRowContext(ctx, `
SELECT r.id, r.workspace_id, r.name, r.plan_step_id, r.created_at
FROM rooms r
JOIN room_members rm ON rm.room_id = r.id
WHERE r.id = ? AND rm.user_id = ?`, roomID, userID).Scan(&room.ID, &room.WorkspaceID, &room.Name, &planStepID, &createdAt); err != nil {
			return mapNotFound(err)
		}
		if planStepID.Valid {
			value := domain.ProjectStepID(planStepID.String)
			room.PlanStepID = &value
		}
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return fmt.Errorf("parse room created_at: %w", err)
		}
		room.CreatedAt = created
		now := time.Now().UTC()
		var pinnedAt any
		if pinned {
			pinnedAt = formatTime(now)
			room.PinnedAt = &now
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO room_preferences (room_id, user_id, pinned, pinned_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(room_id, user_id) DO UPDATE SET
    pinned = excluded.pinned,
    pinned_at = excluded.pinned_at,
    updated_at = excluded.updated_at`, roomID, userID, pinned, pinnedAt, formatTime(now))
		if err != nil {
			return err
		}
		room.Pinned = pinned
		return nil
	})
	return room, err
}

func (r *CollabRepository) AddRoomMember(ctx context.Context, roomID domain.RoomID, actorID, targetID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		var workspaceID domain.WorkspaceID
		if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM rooms WHERE id = ?`, roomID).Scan(&workspaceID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return auth.ErrNotFound
			}
			return err
		}
		workspaceRole, err := workspaceRoleTx(ctx, tx, workspaceID, actorID)
		if err != nil {
			return err
		}
		roomRole, roomRoleErr := roomRoleTx(ctx, tx, roomID, actorID)
		if roomRoleErr != nil && !errors.Is(roomRoleErr, auth.ErrNotFound) {
			return roomRoleErr
		}
		if workspaceRole != domain.WorkspaceOwner && workspaceRole != domain.WorkspaceAdmin && roomRole != domain.RoomOwner && roomRole != domain.RoomAdmin {
			return auth.ErrForbidden
		}
		if _, err := workspaceRoleTx(ctx, tx, workspaceID, targetID); err != nil {
			return auth.ErrNotFound
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO room_members (room_id, user_id, role) VALUES (?, ?, 'member') ON CONFLICT(room_id, user_id) DO NOTHING`, roomID, targetID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return auth.ErrConflict
		}
		return nil
	})
}

func (r *CollabRepository) UpdateRoomMemberRole(ctx context.Context, roomID domain.RoomID, targetID domain.UserID, role domain.RoomRole, actorID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		if role != domain.RoomAdmin && role != domain.RoomMember {
			return auth.ErrForbidden
		}
		var workspaceID domain.WorkspaceID
		if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM rooms WHERE id = ?`, roomID).Scan(&workspaceID); err != nil {
			return mapNotFound(err)
		}
		workspaceRole, err := workspaceRoleTx(ctx, tx, workspaceID, actorID)
		if err != nil {
			return err
		}
		actorRoomRole, err := roomRoleTx(ctx, tx, roomID, actorID)
		if err != nil && workspaceRole != domain.WorkspaceOwner {
			return err
		}
		if targetID == actorID || (actorRoomRole != domain.RoomOwner && workspaceRole != domain.WorkspaceOwner) {
			return auth.ErrForbidden
		}
		var targetRole domain.RoomRole
		if err := tx.QueryRowContext(ctx, `SELECT role FROM room_members WHERE room_id = ? AND user_id = ?`, roomID, targetID).Scan(&targetRole); err != nil {
			return mapNotFound(err)
		}
		if targetRole == domain.RoomOwner {
			return auth.ErrForbidden
		}
		result, err := tx.ExecContext(ctx, `UPDATE room_members SET role = ? WHERE room_id = ? AND user_id = ? AND role != 'owner'`, role, roomID, targetID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		return nil
	})
}

func (r *CollabRepository) RemoveRoomMember(ctx context.Context, roomID domain.RoomID, targetID, actorID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		if targetID == actorID {
			return auth.ErrForbidden
		}
		var workspaceID domain.WorkspaceID
		if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM rooms WHERE id = ?`, roomID).Scan(&workspaceID); err != nil {
			return mapNotFound(err)
		}
		workspaceRole, err := workspaceRoleTx(ctx, tx, workspaceID, actorID)
		if err != nil {
			return err
		}
		actorRoomRole, err := roomRoleTx(ctx, tx, roomID, actorID)
		if err != nil && workspaceRole != domain.WorkspaceOwner && workspaceRole != domain.WorkspaceAdmin {
			return err
		}
		var targetRoomRole domain.RoomRole
		var targetWorkspaceRole domain.WorkspaceRole
		if err := tx.QueryRowContext(ctx, `
SELECT rm.role, wm.role
FROM room_members rm
JOIN rooms r ON r.id = rm.room_id
JOIN workspace_members wm ON wm.workspace_id = r.workspace_id AND wm.user_id = rm.user_id
WHERE rm.room_id = ? AND rm.user_id = ?`, roomID, targetID).Scan(&targetRoomRole, &targetWorkspaceRole); err != nil {
			return mapNotFound(err)
		}
		if targetRoomRole == domain.RoomOwner || targetWorkspaceRole == domain.WorkspaceOwner {
			return auth.ErrForbidden
		}
		canRemove := actorRoomRole == domain.RoomOwner || workspaceRole == domain.WorkspaceOwner || workspaceRole == domain.WorkspaceAdmin
		if actorRoomRole == domain.RoomAdmin && targetRoomRole == domain.RoomMember {
			canRemove = true
		}
		if !canRemove || (actorRoomRole == domain.RoomAdmin && targetRoomRole == domain.RoomAdmin && workspaceRole != domain.WorkspaceOwner && workspaceRole != domain.WorkspaceAdmin) {
			return auth.ErrForbidden
		}
		var active int
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM tasks
WHERE room_id = ? AND target_agent_id IN (SELECT id FROM agents WHERE owner_user_id = ?)
  AND state IN ('queued', 'dispatched', 'running')`, roomID, targetID).Scan(&active); err != nil {
			return err
		}
		if active > 0 {
			return auth.ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_sessions WHERE room_id = ? AND agent_id IN (SELECT id FROM agents WHERE owner_user_id = ?)`, roomID, targetID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM room_agent_settings WHERE room_id = ? AND agent_id IN (SELECT id FROM agents WHERE owner_user_id = ?)`, roomID, targetID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM room_agents WHERE room_id = ? AND agent_id IN (SELECT id FROM agents WHERE owner_user_id = ?)`, roomID, targetID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM room_members WHERE room_id = ? AND user_id = ?`, roomID, targetID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		return nil
	})
}

func (r *CollabRepository) ListRoomMembers(ctx context.Context, roomID domain.RoomID, requesterID domain.UserID) ([]domain.User, error) {
	users := make([]domain.User, 0)
	err := r.store.Read(ctx, func(db *sql.DB) error {
		var member int
		if err := db.QueryRowContext(ctx, `SELECT 1 FROM room_members WHERE room_id = ? AND user_id = ?`, roomID, requesterID).Scan(&member); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return auth.ErrNotFound
			}
			return err
		}
		rows, err := db.QueryContext(ctx, `
SELECT u.id, u.username, u.display_name, u.avatar_url, u.bio, u.current_focus, u.password_hash, u.role, u.created_at, u.updated_at, wm.role, rm.role
FROM users u
JOIN room_members rm ON rm.user_id = u.id
JOIN rooms r ON r.id = rm.room_id
JOIN workspace_members wm ON wm.workspace_id = r.workspace_id AND wm.user_id = u.id
WHERE rm.room_id = ?
ORDER BY CASE rm.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, u.display_name, u.id`, roomID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var user domain.User
			var createdAt, updatedAt string
			if err := rows.Scan(&user.ID, &user.Username, &user.DisplayName, &user.AvatarURL, &user.Bio, &user.CurrentFocus, &user.PasswordHash, &user.Role, &createdAt, &updatedAt, &user.WorkspaceRole, &user.RoomRole); err != nil {
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
			users = append(users, user)
		}
		return rows.Err()
	})
	return users, err
}

func roomRoleTx(ctx context.Context, tx *sql.Tx, roomID domain.RoomID, userID domain.UserID) (domain.RoomRole, error) {
	var role domain.RoomRole
	err := tx.QueryRowContext(ctx, `SELECT role FROM room_members WHERE room_id = ? AND user_id = ?`, roomID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", auth.ErrNotFound
	}
	return role, err
}

func scanRoom(row rowScanner) (domain.Room, error) {
	var room domain.Room
	var createdAt string
	var planStepID, pinnedAt sql.NullString
	if err := row.Scan(&room.ID, &room.WorkspaceID, &room.Name, &planStepID, &createdAt, &room.Pinned, &pinnedAt); err != nil {
		return domain.Room{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.Room{}, fmt.Errorf("parse room created_at: %w", err)
	}
	room.CreatedAt = created
	if planStepID.Valid {
		value := domain.ProjectStepID(planStepID.String)
		room.PlanStepID = &value
	}
	if pinnedAt.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, pinnedAt.String)
		if err != nil {
			return domain.Room{}, fmt.Errorf("parse room pinned_at: %w", err)
		}
		room.PinnedAt = &parsed
	}
	return room, nil
}
