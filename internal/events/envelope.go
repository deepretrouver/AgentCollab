package events

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type SenderKind string

const (
	SenderUser   SenderKind = "user"
	SenderAgent  SenderKind = "agent"
	SenderRunner SenderKind = "runner"
	SenderSystem SenderKind = "system"
)

type Sender struct {
	Kind SenderKind `json:"kind"`
	ID   string     `json:"id"`
}

func (s Sender) Validate() error {
	switch s.Kind {
	case SenderUser, SenderAgent, SenderRunner, SenderSystem:
	default:
		return fmt.Errorf("invalid sender kind %q", s.Kind)
	}
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("sender id is required")
	}
	return nil
}

type Envelope struct {
	EventID     string          `json:"event_id"`
	Type        string          `json:"type"`
	WorkspaceID string          `json:"workspace_id"`
	RoomID      string          `json:"room_id"`
	Sequence    uint64          `json:"sequence"`
	Sender      Sender          `json:"sender"`
	OccurredAt  time.Time       `json:"occurred_at"`
	Payload     json.RawMessage `json:"payload"`
}

func (e Envelope) Validate() error {
	for name, value := range map[string]string{
		"event_id":     e.EventID,
		"type":         e.Type,
		"workspace_id": e.WorkspaceID,
		"room_id":      e.RoomID,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if e.Sequence == 0 {
		return fmt.Errorf("sequence must be greater than zero")
	}
	if err := e.Sender.Validate(); err != nil {
		return err
	}
	if e.OccurredAt.IsZero() {
		return fmt.Errorf("occurred_at is required")
	}
	if len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return fmt.Errorf("payload must be valid JSON")
	}
	return nil
}
