package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
)

func (r *CollabRepository) CreateMessage(ctx context.Context, message domain.Message, userID domain.UserID) (domain.Message, error) {
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
SELECT r.workspace_id
FROM rooms r
JOIN room_members rm ON rm.room_id = r.id
WHERE r.id = ? AND rm.user_id = ?`, message.RoomID, userID).Scan(&message.WorkspaceID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return auth.ErrNotFound
			}
			return err
		}
		if err := normalizeMessageThreadTx(ctx, tx, &message); err != nil {
			return err
		}
		agents, err := roomRoutingAgentsTx(ctx, tx, message.RoomID)
		if err != nil {
			return err
		}
		if topic, brainstorm := brainstormTopic(message.Body); brainstorm {
			if topic == "" {
				return fmt.Errorf("%w: brainstorm topic is required", auth.ErrInvalidInput)
			}
			var coordinator *routingAgent
			for index := range agents {
				if agents[index].ResponseMode == domain.AgentResponseCoordinator && canUserInvokeAgent(userID, agents[index]) {
					coordinator = &agents[index]
					break
				}
			}
			if coordinator == nil {
				return auth.ErrCoordinatorRequired
			}
			message.MentionAgentIDs = []domain.AgentID{coordinator.ID}
			metadataValues := map[string]any{}
			_ = json.Unmarshal(message.Metadata, &metadataValues)
			metadataValues["workflow"] = "brainstorm"
			metadataValues["topic"] = topic
			metadataValues["coordinator_agent_id"] = coordinator.ID
			metadata, _ := json.Marshal(metadataValues)
			message.Metadata = metadata
		} else {
			message.MentionAgentIDs = validMentionIDs(message.MentionAgentIDs, agents, userID)
			message.MentionAgentIDs = expandGroupMention(message.MentionAgentIDs, message.Body, agents, userID)
		}
		message.Sequence, err = allocateSequence(ctx, tx, message.WorkspaceID)
		if err != nil {
			return err
		}
		mentionsJSON, _ := json.Marshal(message.MentionAgentIDs)
		metadata := message.Metadata
		if len(metadata) == 0 || !json.Valid(metadata) {
			metadata = json.RawMessage(`{}`)
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO messages (message_id, workspace_id, room_id, sequence, sender_kind, sender_id, kind, body,
                      mention_agent_ids_json, reply_to_message_id, root_message_id, metadata_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, message.ID, message.WorkspaceID, message.RoomID, message.Sequence,
			message.SenderKind, message.SenderID, message.Kind, message.Body, string(mentionsJSON), message.ReplyToMessageID,
			message.RootMessageID, string(metadata), formatTime(message.CreatedAt))
		if err != nil {
			return err
		}
		if err := attachMessageFilesTx(ctx, tx, &message, userID); err != nil {
			return err
		}
		if isConversationBoundary(message.Body) {
			_, err = tx.ExecContext(ctx, `DELETE FROM agent_sessions WHERE room_id = ?`, message.RoomID)
			return err
		}

		rootMessageID := message.ID
		if message.RootMessageID != nil {
			rootMessageID = *message.RootMessageID
		}
		targets := selectUserMessageTargets(message.MentionAgentIDs, agents, userID)
		if message.RootMessageID != nil {
			targets = selectExplicitMessageTargets(message.MentionAgentIDs, agents, userID)
		}
		for _, target := range targets {
			executionSource := messageTaskExecutionSource(message.Metadata)
			if err := createRoutedTaskTx(ctx, tx, routedTaskInput{
				WorkspaceID:     message.WorkspaceID,
				RoomID:          message.RoomID,
				RootMessageID:   rootMessageID,
				SourceMessageID: message.ID,
				CreatorKind:     domain.TaskCreatorUser,
				CreatorID:       string(userID),
				Target:          target,
				SourceBody:      message.Body,
				Depth:           0,
				CreatedAt:       message.CreatedAt,
				ExecutionSource: executionSource,
			}, agents); err != nil {
				return err
			}
		}
		if planStepID := messageProjectStepID(message.Metadata); planStepID != nil && message.RootMessageID == nil {
			if _, err := ensureWorkItemForRootTx(ctx, tx, message.WorkspaceID, message.RoomID, message.ID, "项目会话", message.CreatedAt); err != nil {
				return err
			}
		}
		return nil
	})
	return message, err
}

func messageTaskExecutionSource(metadata json.RawMessage) domain.TaskExecutionSource {
	if len(metadata) == 0 || !json.Valid(metadata) {
		return ""
	}
	var value struct {
		ExecutionSource domain.TaskExecutionSource `json:"execution_source"`
	}
	if json.Unmarshal(metadata, &value) != nil {
		return ""
	}
	return value.ExecutionSource
}

func messageProjectStepID(metadata json.RawMessage) *domain.ProjectStepID {
	if len(metadata) == 0 || !json.Valid(metadata) {
		return nil
	}
	var value struct {
		PlanStepID *domain.ProjectStepID `json:"plan_step_id"`
	}
	if json.Unmarshal(metadata, &value) != nil {
		return nil
	}
	return value.PlanStepID
}

// normalizeMessageThreadTx accepts only links to messages in the same room and
// keeps a reply attached to the original human root. This is the server-side
// guard that prevents a client from making a reply appear in an unrelated group.
func normalizeMessageThreadTx(ctx context.Context, tx *sql.Tx, message *domain.Message) error {
	if message.ReplyToMessageID == nil && message.RootMessageID == nil {
		return nil
	}
	if message.ReplyToMessageID != nil {
		var parentRoot sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT root_message_id FROM messages WHERE message_id = ? AND room_id = ?`, *message.ReplyToMessageID, message.RoomID).Scan(&parentRoot); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return auth.ErrNotFound
			}
			return err
		}
		root := *message.ReplyToMessageID
		if parentRoot.Valid {
			root = domain.MessageID(parentRoot.String)
		}
		if message.RootMessageID != nil && *message.RootMessageID != root {
			return auth.ErrInvalidInput
		}
		message.RootMessageID = &root
	}
	if message.RootMessageID == nil {
		return nil
	}
	var senderKind domain.SenderKind
	if err := tx.QueryRowContext(ctx, `SELECT sender_kind FROM messages WHERE message_id = ? AND room_id = ?`, *message.RootMessageID, message.RoomID).Scan(&senderKind); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return auth.ErrNotFound
		}
		return err
	}
	if senderKind != domain.MessageSenderUser {
		return auth.ErrInvalidInput
	}
	return nil
}

func (r *CollabRepository) ListMessages(ctx context.Context, roomID domain.RoomID, userID domain.UserID, after uint64, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	messages := make([]domain.Message, 0)
	err := r.store.Read(ctx, func(db *sql.DB) error {
		var member int
		if err := db.QueryRowContext(ctx, `SELECT 1 FROM room_members WHERE room_id = ? AND user_id = ?`, roomID, userID).Scan(&member); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return auth.ErrNotFound
			}
			return err
		}
		rows, err := db.QueryContext(ctx, `
SELECT message_id, workspace_id, room_id, sequence, sender_kind, sender_id, kind, body,
       mention_agent_ids_json, reply_to_message_id, root_message_id, metadata_json, created_at
FROM messages
WHERE room_id = ? AND sequence > ?
ORDER BY sequence
LIMIT ?`, roomID, after, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			message, err := scanMessage(rows)
			if err != nil {
				return err
			}
			messages = append(messages, message)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		return loadMessageAttachments(ctx, db, roomID, messages)
	})
	return messages, err
}

func scanMessage(row rowScanner) (domain.Message, error) {
	var message domain.Message
	var mentionsJSON, metadataJSON, createdAt string
	var replyTo, root sql.NullString
	if err := row.Scan(&message.ID, &message.WorkspaceID, &message.RoomID, &message.Sequence, &message.SenderKind, &message.SenderID,
		&message.Kind, &message.Body, &mentionsJSON, &replyTo, &root, &metadataJSON, &createdAt); err != nil {
		return domain.Message{}, err
	}
	_ = json.Unmarshal([]byte(mentionsJSON), &message.MentionAgentIDs)
	if message.MentionAgentIDs == nil {
		message.MentionAgentIDs = []domain.AgentID{}
	}
	if replyTo.Valid {
		value := domain.MessageID(replyTo.String)
		message.ReplyToMessageID = &value
	}
	if root.Valid {
		value := domain.MessageID(root.String)
		message.RootMessageID = &value
	}
	if json.Valid([]byte(metadataJSON)) && strings.TrimSpace(metadataJSON) != "{}" {
		message.Metadata = json.RawMessage(metadataJSON)
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.Message{}, fmt.Errorf("parse message created_at: %w", err)
	}
	message.CreatedAt = created
	return message, nil
}
