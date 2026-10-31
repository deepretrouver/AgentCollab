package sqlite

import (
	"context"
	"database/sql"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
)

func (r *CollabRepository) WorkspaceForRoomUser(ctx context.Context, roomID domain.RoomID, userID domain.UserID) (domain.WorkspaceID, error) {
	var workspaceID domain.WorkspaceID
	err := r.store.Read(ctx, func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `
SELECT rooms.workspace_id FROM rooms
JOIN room_members ON room_members.room_id = rooms.id
WHERE rooms.id = ? AND room_members.user_id = ?`, roomID, userID).Scan(&workspaceID)
	})
	return workspaceID, mapNotFound(err)
}

func (r *CollabRepository) CreateKnowledgeAsset(ctx context.Context, asset domain.KnowledgeAsset, userID domain.UserID) (domain.KnowledgeAsset, error) {
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		if asset.RoomID == nil {
			if _, err := workspaceRoleTx(ctx, tx, asset.WorkspaceID, userID); err != nil {
				return err
			}
		} else {
			var workspaceID domain.WorkspaceID
			if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM rooms WHERE id = ?`, *asset.RoomID).Scan(&workspaceID); err != nil {
				return mapNotFound(err)
			}
			if workspaceID != asset.WorkspaceID {
				return auth.ErrForbidden
			}
			if _, err := roomRoleTx(ctx, tx, *asset.RoomID, userID); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO knowledge_assets
    (id, workspace_id, room_id, uploader_user_id, filename, content_type, size_bytes, sha256, relative_path, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, asset.ID, asset.WorkspaceID, asset.RoomID, userID,
			asset.Filename, asset.ContentType, asset.SizeBytes, asset.SHA256, asset.RelativePath, formatTime(asset.CreatedAt))
		return err
	})
	asset.UploaderUserID = userID
	asset.CanDelete = true
	if asset.RoomID == nil {
		asset.Scope = domain.KnowledgeWorkspace
	} else {
		asset.Scope = domain.KnowledgeRoom
	}
	asset.DownloadURL = knowledgeDownloadURL(asset)
	return asset, err
}

func (r *CollabRepository) ListWorkspaceKnowledge(ctx context.Context, workspaceID domain.WorkspaceID, userID domain.UserID) ([]domain.KnowledgeAsset, error) {
	assets := make([]domain.KnowledgeAsset, 0)
	err := r.store.Read(ctx, func(db *sql.DB) error {
		var role domain.WorkspaceRole
		if err := db.QueryRowContext(ctx, `SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, userID).Scan(&role); err != nil {
			return mapNotFound(err)
		}
		rows, err := db.QueryContext(ctx, knowledgeSelect+` WHERE ka.workspace_id = ? AND ka.room_id IS NULL ORDER BY ka.created_at DESC, ka.id`, workspaceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var asset domain.KnowledgeAsset
			if err := scanKnowledgeAsset(rows, &asset); err != nil {
				return err
			}
			asset.CanDelete = asset.UploaderUserID == userID || role == domain.WorkspaceOwner || role == domain.WorkspaceAdmin
			assets = append(assets, asset)
		}
		return rows.Err()
	})
	return assets, err
}

func (r *CollabRepository) ListRoomKnowledge(ctx context.Context, roomID domain.RoomID, userID domain.UserID) ([]domain.KnowledgeAsset, error) {
	assets := make([]domain.KnowledgeAsset, 0)
	err := r.store.Read(ctx, func(db *sql.DB) error {
		var roomRole domain.RoomRole
		var workspaceRole domain.WorkspaceRole
		if err := db.QueryRowContext(ctx, `
SELECT rm.role, wm.role FROM rooms r
JOIN room_members rm ON rm.room_id = r.id AND rm.user_id = ?
JOIN workspace_members wm ON wm.workspace_id = r.workspace_id AND wm.user_id = ?
WHERE r.id = ?`, userID, userID, roomID).Scan(&roomRole, &workspaceRole); err != nil {
			return mapNotFound(err)
		}
		rows, err := db.QueryContext(ctx, knowledgeSelect+` WHERE ka.room_id = ? ORDER BY ka.created_at DESC, ka.id`, roomID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var asset domain.KnowledgeAsset
			if err := scanKnowledgeAsset(rows, &asset); err != nil {
				return err
			}
			asset.CanDelete = asset.UploaderUserID == userID || roomRole == domain.RoomOwner || roomRole == domain.RoomAdmin || workspaceRole == domain.WorkspaceOwner || workspaceRole == domain.WorkspaceAdmin
			assets = append(assets, asset)
		}
		return rows.Err()
	})
	return assets, err
}

func (r *CollabRepository) KnowledgeAssetForUser(ctx context.Context, assetID domain.KnowledgeAssetID, userID domain.UserID) (domain.KnowledgeAsset, error) {
	var asset domain.KnowledgeAsset
	err := r.store.Read(ctx, func(db *sql.DB) error {
		return scanKnowledgeAsset(db.QueryRowContext(ctx, knowledgeSelect+`
WHERE ka.id = ? AND (
    (ka.room_id IS NULL AND EXISTS (SELECT 1 FROM workspace_members wm WHERE wm.workspace_id = ka.workspace_id AND wm.user_id = ?))
 OR (ka.room_id IS NOT NULL AND EXISTS (SELECT 1 FROM room_members rm WHERE rm.room_id = ka.room_id AND rm.user_id = ?))
)`, assetID, userID, userID), &asset)
	})
	return asset, mapNotFound(err)
}

func (r *CollabRepository) DeleteKnowledgeAsset(ctx context.Context, assetID domain.KnowledgeAssetID, userID domain.UserID) (domain.KnowledgeAsset, error) {
	var asset domain.KnowledgeAsset
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		if err := scanKnowledgeAsset(tx.QueryRowContext(ctx, knowledgeSelect+` WHERE ka.id = ?`, assetID), &asset); err != nil {
			return mapNotFound(err)
		}
		allowed := asset.UploaderUserID == userID
		workspaceRole, workspaceErr := workspaceRoleTx(ctx, tx, asset.WorkspaceID, userID)
		if workspaceErr == nil && (workspaceRole == domain.WorkspaceOwner || workspaceRole == domain.WorkspaceAdmin) {
			allowed = true
		}
		if asset.RoomID != nil {
			roomRole, roomErr := roomRoleTx(ctx, tx, *asset.RoomID, userID)
			if roomErr == nil && (roomRole == domain.RoomOwner || roomRole == domain.RoomAdmin) {
				allowed = true
			}
		}
		if !allowed {
			return auth.ErrForbidden
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM knowledge_assets WHERE id = ?`, assetID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		return nil
	})
	return asset, err
}

func (r *CollabRepository) KnowledgeAssetForRunnerTask(ctx context.Context, taskID domain.TaskID, assetID domain.KnowledgeAssetID, runnerID domain.RunnerID) (domain.KnowledgeAsset, error) {
	var asset domain.KnowledgeAsset
	err := r.store.Read(ctx, func(db *sql.DB) error {
		return scanKnowledgeAsset(db.QueryRowContext(ctx, knowledgeSelect+`
JOIN tasks t ON t.workspace_id = ka.workspace_id AND (ka.room_id IS NULL OR ka.room_id = t.room_id)
JOIN agents a ON a.id = t.target_agent_id
WHERE t.id = ? AND ka.id = ? AND a.runner_id = ?`, taskID, assetID, runnerID), &asset)
	})
	return asset, mapNotFound(err)
}

func (r *CollabRepository) ListProjectFileScopes(ctx context.Context) ([]domain.ProjectFileScope, error) {
	scopes := make([]domain.ProjectFileScope, 0)
	err := r.store.Read(ctx, func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `
SELECT w.id, r.id FROM workspaces w LEFT JOIN rooms r ON r.workspace_id = w.id
ORDER BY w.id, r.id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var workspaceID domain.WorkspaceID
			var roomID sql.NullString
			if err := rows.Scan(&workspaceID, &roomID); err != nil {
				return err
			}
			scope := domain.ProjectFileScope{WorkspaceID: workspaceID}
			if roomID.Valid {
				value := domain.RoomID(roomID.String)
				scope.RoomID = &value
			}
			scopes = append(scopes, scope)
		}
		return rows.Err()
	})
	return scopes, err
}

func listKnowledgeAssetsForTaskTx(ctx context.Context, tx *sql.Tx, workspaceID domain.WorkspaceID, roomID domain.RoomID) ([]domain.KnowledgeAsset, error) {
	rows, err := tx.QueryContext(ctx, knowledgeSelect+`
WHERE ka.workspace_id = ? AND (ka.room_id IS NULL OR ka.room_id = ?)
ORDER BY CASE WHEN ka.room_id IS NULL THEN 0 ELSE 1 END, ka.created_at, ka.id`, workspaceID, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	assets := make([]domain.KnowledgeAsset, 0)
	for rows.Next() {
		var asset domain.KnowledgeAsset
		if err := scanKnowledgeAsset(rows, &asset); err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	return assets, rows.Err()
}

const knowledgeSelect = `
SELECT ka.id, ka.workspace_id, ka.room_id, ka.uploader_user_id, u.display_name,
       ka.filename, ka.content_type, ka.size_bytes, ka.sha256, ka.relative_path, ka.created_at
FROM knowledge_assets ka
JOIN users u ON u.id = ka.uploader_user_id`

func scanKnowledgeAsset(row rowScanner, asset *domain.KnowledgeAsset) error {
	var roomID sql.NullString
	var createdAt string
	if err := row.Scan(&asset.ID, &asset.WorkspaceID, &roomID, &asset.UploaderUserID, &asset.UploaderName,
		&asset.Filename, &asset.ContentType, &asset.SizeBytes, &asset.SHA256, &asset.RelativePath, &createdAt); err != nil {
		return err
	}
	if roomID.Valid {
		value := domain.RoomID(roomID.String)
		asset.RoomID = &value
		asset.Scope = domain.KnowledgeRoom
	} else {
		asset.Scope = domain.KnowledgeWorkspace
	}
	asset.DownloadURL = knowledgeDownloadURL(*asset)
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return err
	}
	asset.CreatedAt = created
	return nil
}

func knowledgeDownloadURL(asset domain.KnowledgeAsset) string {
	if asset.RoomID != nil {
		return "/api/v1/rooms/" + string(*asset.RoomID) + "/knowledge/" + string(asset.ID)
	}
	return "/api/v1/workspaces/" + string(asset.WorkspaceID) + "/knowledge/" + string(asset.ID)
}
