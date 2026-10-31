package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/events"
	"agentcollab.local/agentcollab/internal/idgen"
)

func (h *CollabHandlers) createMessage(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var input struct {
		Body             string                     `json:"body"`
		MentionAgentIDs  []domain.AgentID           `json:"mention_agent_ids"`
		AttachmentIDs    []domain.AttachmentID      `json:"attachment_ids"`
		ReplyToMessageID *domain.MessageID          `json:"reply_to_message_id"`
		RootMessageID    *domain.MessageID          `json:"root_message_id"`
		PlanStepID       *domain.ProjectStepID      `json:"plan_step_id"`
		ExecutionSource  domain.TaskExecutionSource `json:"execution_source"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	input.Body = strings.TrimSpace(input.Body)
	if len(input.AttachmentIDs) > 5 {
		writeError(w, http.StatusBadRequest, "invalid_request", "每条消息最多发送 5 个附件")
		return
	}
	if input.Body == "" && len(input.AttachmentIDs) > 0 {
		input.Body = "请处理随附文件。"
	}
	if input.Body == "" || len(input.Body) > 20_000 {
		writeError(w, http.StatusBadRequest, "invalid_request", "message body must be 1-20000 characters")
		return
	}
	if strings.EqualFold(input.Body, "/brainstorm") {
		writeError(w, http.StatusBadRequest, "brainstorm_topic_required", "请在 /brainstorm 后填写专题议题")
		return
	}
	if !input.ExecutionSource.Valid() || (input.ExecutionSource == domain.TaskExecutionResponsibleLocal && input.PlanStepID == nil) {
		writeError(w, http.StatusBadRequest, "invalid_request", "选择负责人本机进展时必须同时选择项目步骤")
		return
	}
	id, err := idgen.New("msg")
	if err != nil {
		h.writeError(w, err)
		return
	}
	metadata := map[string]any{}
	if input.PlanStepID != nil {
		metadata["plan_step_id"] = *input.PlanStepID
	}
	if input.ExecutionSource != "" {
		metadata["execution_source"] = input.ExecutionSource
	}
	metadataJSON, _ := json.Marshal(metadata)
	message := domain.Message{
		ID:               domain.MessageID(id),
		RoomID:           domain.RoomID(r.PathValue("roomID")),
		SenderKind:       domain.MessageSenderUser,
		SenderID:         string(user.ID),
		Kind:             domain.MessageChat,
		Body:             input.Body,
		MentionAgentIDs:  input.MentionAgentIDs,
		AttachmentIDs:    input.AttachmentIDs,
		ReplyToMessageID: input.ReplyToMessageID,
		RootMessageID:    input.RootMessageID,
		Metadata:         metadataJSON,
		CreatedAt:        time.Now().UTC(),
	}
	message, err = h.repository.CreateMessage(r.Context(), message, user.ID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	if h.publisher != nil {
		payload, err := json.Marshal(message)
		if err == nil {
			h.publisher.Publish(events.Envelope{
				EventID:     string(message.ID),
				Type:        "message.created",
				WorkspaceID: string(message.WorkspaceID),
				RoomID:      string(message.RoomID),
				Sequence:    message.Sequence,
				Sender:      events.Sender{Kind: events.SenderUser, ID: message.SenderID},
				OccurredAt:  message.CreatedAt,
				Payload:     payload,
			})
		}
	}
	writeJSON(w, http.StatusCreated, message)
}

func (h *CollabHandlers) listMessages(w http.ResponseWriter, r *http.Request) {
	h.writeMessages(w, r, "messages")
}

func (h *CollabHandlers) timeline(w http.ResponseWriter, r *http.Request) {
	h.writeMessages(w, r, "messages")
}

func (h *CollabHandlers) writeMessages(w http.ResponseWriter, r *http.Request, key string) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	after, err := parseUintQuery(r, "after", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "after must be an unsigned integer")
		return
	}
	limitValue, err := parseUintQuery(r, "limit", 50)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "limit must be an unsigned integer")
		return
	}
	messages, err := h.repository.ListMessages(r.Context(), domain.RoomID(r.PathValue("roomID")), user.ID, after, int(limitValue))
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{key: messages})
}

func parseUintQuery(r *http.Request, name string, defaultValue uint64) (uint64, error) {
	value := strings.TrimSpace(r.URL.Query().Get(name))
	if value == "" {
		return defaultValue, nil
	}
	return strconv.ParseUint(value, 10, 64)
}
