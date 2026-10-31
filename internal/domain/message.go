package domain

import (
	"encoding/json"
	"time"
)

type MessageID string

type SenderKind string

const (
	MessageSenderUser   SenderKind = "user"
	MessageSenderAgent  SenderKind = "agent"
	MessageSenderSystem SenderKind = "system"
)

type MessageKind string
type AttachmentID string

const (
	MessageChat     MessageKind = "chat"
	MessageSystem   MessageKind = "system"
	MessageTask     MessageKind = "task"
	MessageProgress MessageKind = "progress"
	MessageDelivery MessageKind = "delivery"
)

type Message struct {
	ID               MessageID       `json:"id"`
	WorkspaceID      WorkspaceID     `json:"workspace_id"`
	RoomID           RoomID          `json:"room_id"`
	Sequence         uint64          `json:"sequence"`
	SenderKind       SenderKind      `json:"sender_kind"`
	SenderID         string          `json:"sender_id"`
	Kind             MessageKind     `json:"kind"`
	Body             string          `json:"body"`
	MentionAgentIDs  []AgentID       `json:"mention_agent_ids,omitempty"`
	ReplyToMessageID *MessageID      `json:"reply_to_message_id,omitempty"`
	RootMessageID    *MessageID      `json:"root_message_id,omitempty"`
	Metadata         json.RawMessage `json:"metadata,omitempty"`
	AttachmentIDs    []AttachmentID  `json:"-"`
	Attachments      []Attachment    `json:"attachments,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
}

type Attachment struct {
	ID             AttachmentID `json:"id"`
	WorkspaceID    WorkspaceID  `json:"workspace_id"`
	RoomID         RoomID       `json:"room_id"`
	UploaderUserID UserID       `json:"uploader_user_id"`
	MessageID      *MessageID   `json:"message_id,omitempty"`
	Filename       string       `json:"filename"`
	ContentType    string       `json:"content_type"`
	SizeBytes      int64        `json:"size_bytes"`
	SHA256         string       `json:"sha256"`
	RelativePath   string       `json:"-"`
	DownloadURL    string       `json:"download_url,omitempty"`
	LocalPath      string       `json:"-"`
	CreatedAt      time.Time    `json:"created_at"`
}
