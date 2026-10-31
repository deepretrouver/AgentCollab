package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/events"
)

func (r *CollabRepository) RoomWorkspace(ctx context.Context, roomID domain.RoomID, userID domain.UserID) (domain.WorkspaceID, error) {
	var workspaceID domain.WorkspaceID
	err := r.store.Read(ctx, func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `
SELECT rooms.workspace_id
FROM rooms
JOIN room_members ON room_members.room_id = rooms.id
WHERE rooms.id = ? AND room_members.user_id = ?`, roomID, userID).Scan(&workspaceID)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", auth.ErrNotFound
	}
	return workspaceID, err
}

func (r *CollabRepository) ReplayRoomEvents(ctx context.Context, roomID domain.RoomID, userID domain.UserID, after uint64, limit int) ([]events.Envelope, error) {
	messages, err := r.ListMessages(ctx, roomID, userID, after, limit)
	if err != nil {
		return nil, err
	}
	result := make([]events.Envelope, 0, len(messages))
	for _, message := range messages {
		payload, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}
		result = append(result, events.Envelope{
			EventID:     string(message.ID),
			Type:        "message.created",
			WorkspaceID: string(message.WorkspaceID),
			RoomID:      string(message.RoomID),
			Sequence:    message.Sequence,
			Sender: events.Sender{
				Kind: events.SenderKind(message.SenderKind),
				ID:   message.SenderID,
			},
			OccurredAt: message.CreatedAt,
			Payload:    payload,
		})
	}
	return result, nil
}
