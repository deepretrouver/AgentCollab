package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
)

func (r *CollabRepository) CreateAttachment(ctx context.Context, attachment domain.Attachment, userID domain.UserID) (domain.Attachment, error) {
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		var workspaceID domain.WorkspaceID
		if err := tx.QueryRowContext(ctx, `
SELECT rooms.workspace_id FROM rooms
JOIN room_members ON room_members.room_id = rooms.id
WHERE rooms.id = ? AND room_members.user_id = ?`, attachment.RoomID, userID).Scan(&workspaceID); err != nil {
			return mapNotFound(err)
		}
		if attachment.WorkspaceID != "" && attachment.WorkspaceID != workspaceID {
			return auth.ErrForbidden
		}
		attachment.WorkspaceID = workspaceID
		_, err := tx.ExecContext(ctx, `
INSERT INTO message_attachments
    (id, workspace_id, room_id, uploader_user_id, filename, content_type, size_bytes, sha256, relative_path, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, attachment.ID, attachment.WorkspaceID, attachment.RoomID, userID,
			attachment.Filename, attachment.ContentType, attachment.SizeBytes, attachment.SHA256, attachment.RelativePath, formatTime(attachment.CreatedAt))
		return err
	})
	return attachment, err
}

func (r *CollabRepository) CreateTaskArtifact(ctx context.Context, taskID domain.TaskID, runnerID domain.RunnerID, attachment domain.Attachment) (domain.Attachment, error) {
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		var state domain.TaskState
		if err := tx.QueryRowContext(ctx, `SELECT t.workspace_id, t.room_id, a.owner_user_id, t.state
FROM tasks t JOIN agents a ON a.id = t.target_agent_id
WHERE t.id = ? AND a.runner_id = ?`, taskID, runnerID).Scan(&attachment.WorkspaceID, &attachment.RoomID, &attachment.UploaderUserID, &state); err != nil {
			return mapNotFound(err)
		}
		if state != domain.TaskDispatched && state != domain.TaskRunning {
			return auth.ErrConflict
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM message_attachments WHERE task_id = ?`, taskID).Scan(&count); err != nil {
			return err
		}
		if count >= 5 {
			return auth.ErrInvalidInput
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO message_attachments
    (id, workspace_id, room_id, uploader_user_id, task_id, filename, content_type, size_bytes, sha256, relative_path, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, attachment.ID, attachment.WorkspaceID, attachment.RoomID, attachment.UploaderUserID, taskID,
			attachment.Filename, attachment.ContentType, attachment.SizeBytes, attachment.SHA256, attachment.RelativePath, formatTime(attachment.CreatedAt))
		return err
	})
	return attachment, err
}

func (r *CollabRepository) AttachmentForUser(ctx context.Context, attachmentID domain.AttachmentID, userID domain.UserID) (domain.Attachment, error) {
	var attachment domain.Attachment
	err := r.store.Read(ctx, func(db *sql.DB) error {
		return scanAttachment(db.QueryRowContext(ctx, `
SELECT ma.id, ma.workspace_id, ma.room_id, ma.uploader_user_id, ma.message_id, ma.filename,
       ma.content_type, ma.size_bytes, ma.sha256, ma.relative_path, ma.created_at
FROM message_attachments ma
JOIN room_members rm ON rm.room_id = ma.room_id
WHERE ma.id = ? AND ma.message_id IS NOT NULL AND rm.user_id = ?`, attachmentID, userID), &attachment)
	})
	return attachment, mapNotFound(err)
}

func (r *CollabRepository) AttachmentForRunnerTask(ctx context.Context, taskID domain.TaskID, attachmentID domain.AttachmentID, runnerID domain.RunnerID) (domain.Attachment, error) {
	var attachment domain.Attachment
	err := r.store.Read(ctx, func(db *sql.DB) error {
		return scanAttachment(db.QueryRowContext(ctx, `
SELECT ma.id, ma.workspace_id, ma.room_id, ma.uploader_user_id, ma.message_id, ma.filename,
       ma.content_type, ma.size_bytes, ma.sha256, ma.relative_path, ma.created_at
FROM message_attachments ma
JOIN tasks t ON t.source_message_id = ma.message_id AND t.room_id = ma.room_id
JOIN agents a ON a.id = t.target_agent_id
WHERE t.id = ? AND ma.id = ? AND a.runner_id = ?`, taskID, attachmentID, runnerID), &attachment)
	})
	return attachment, mapNotFound(err)
}

func attachMessageFilesTx(ctx context.Context, tx *sql.Tx, message *domain.Message, userID domain.UserID) error {
	if len(message.AttachmentIDs) == 0 {
		message.Attachments = nil
		return nil
	}
	seen := make(map[domain.AttachmentID]bool, len(message.AttachmentIDs))
	for _, attachmentID := range message.AttachmentIDs {
		if strings.TrimSpace(string(attachmentID)) == "" || seen[attachmentID] {
			return fmt.Errorf("%w: attachment ids must be non-empty and unique", auth.ErrInvalidInput)
		}
		seen[attachmentID] = true
		result, err := tx.ExecContext(ctx, `
UPDATE message_attachments SET message_id = ?
WHERE id = ? AND room_id = ? AND uploader_user_id = ? AND message_id IS NULL`, message.ID, attachmentID, message.RoomID, userID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return fmt.Errorf("%w: attachment is unavailable", auth.ErrInvalidInput)
		}
	}
	attachments, err := listAttachmentsTx(ctx, tx, message.ID)
	if err != nil {
		return err
	}
	message.Attachments = attachments
	return nil
}

func listAttachmentsTx(ctx context.Context, tx *sql.Tx, messageID domain.MessageID) ([]domain.Attachment, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, workspace_id, room_id, uploader_user_id, message_id, filename, content_type,
       size_bytes, sha256, relative_path, created_at
FROM message_attachments WHERE message_id = ? ORDER BY created_at, id`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attachments := make([]domain.Attachment, 0)
	for rows.Next() {
		var attachment domain.Attachment
		if err := scanAttachment(rows, &attachment); err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}
	return attachments, rows.Err()
}

func loadMessageAttachments(ctx context.Context, db *sql.DB, roomID domain.RoomID, messages []domain.Message) error {
	if len(messages) == 0 {
		return nil
	}
	messageIndexes := make(map[domain.MessageID]int, len(messages))
	placeholders := make([]string, 0, len(messages))
	args := make([]any, 0, len(messages)+1)
	args = append(args, roomID)
	for index := range messages {
		messageIndexes[messages[index].ID] = index
		placeholders = append(placeholders, "?")
		args = append(args, messages[index].ID)
	}
	rows, err := db.QueryContext(ctx, `
SELECT id, workspace_id, room_id, uploader_user_id, message_id, filename, content_type,
       size_bytes, sha256, relative_path, created_at
FROM message_attachments
WHERE room_id = ? AND message_id IN (`+strings.Join(placeholders, ",")+`)
ORDER BY created_at, id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var attachment domain.Attachment
		if err := scanAttachment(rows, &attachment); err != nil {
			return err
		}
		if attachment.MessageID != nil {
			if index, ok := messageIndexes[*attachment.MessageID]; ok {
				messages[index].Attachments = append(messages[index].Attachments, attachment)
			}
		}
	}
	return rows.Err()
}

func scanAttachment(row rowScanner, attachment *domain.Attachment) error {
	var messageID sql.NullString
	var createdAt string
	if err := row.Scan(&attachment.ID, &attachment.WorkspaceID, &attachment.RoomID, &attachment.UploaderUserID,
		&messageID, &attachment.Filename, &attachment.ContentType, &attachment.SizeBytes, &attachment.SHA256,
		&attachment.RelativePath, &createdAt); err != nil {
		return err
	}
	if messageID.Valid {
		value := domain.MessageID(messageID.String)
		attachment.MessageID = &value
		attachment.DownloadURL = "/api/v1/rooms/" + string(attachment.RoomID) + "/attachments/" + string(attachment.ID)
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return err
	}
	attachment.CreatedAt = created
	return nil
}
