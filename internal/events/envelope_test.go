package events

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	want := Envelope{
		EventID:     "evt_0000000000001_aaaaaaaaaaaaaaaa",
		Type:        "task.progress",
		WorkspaceID: "ws_0000000000001_bbbbbbbbbbbbbbbb",
		RoomID:      "room_0000000000001_cccccccccccccccc",
		Sequence:    42,
		Sender: Sender{
			Kind: SenderAgent,
			ID:   "agent_0000000000001_dddddddddddddddd",
		},
		OccurredAt: time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC),
		Payload:    json.RawMessage(`{"message":"working"}`),
	}
	if err := want.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got Envelope
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.EventID != want.EventID || got.Type != want.Type || got.Sequence != want.Sequence || got.Sender != want.Sender || string(got.Payload) != string(want.Payload) {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}

func TestEnvelopeValidation(t *testing.T) {
	valid := Envelope{
		EventID:     "evt_1",
		Type:        "message.created",
		WorkspaceID: "ws_1",
		RoomID:      "room_1",
		Sequence:    1,
		Sender:      Sender{Kind: SenderUser, ID: "user_1"},
		OccurredAt:  time.Now().UTC(),
		Payload:     json.RawMessage(`{}`),
	}
	tests := []struct {
		name   string
		mutate func(*Envelope)
	}{
		{"missing event id", func(e *Envelope) { e.EventID = "" }},
		{"missing type", func(e *Envelope) { e.Type = "" }},
		{"missing workspace", func(e *Envelope) { e.WorkspaceID = "" }},
		{"missing room", func(e *Envelope) { e.RoomID = "" }},
		{"zero sequence", func(e *Envelope) { e.Sequence = 0 }},
		{"invalid sender", func(e *Envelope) { e.Sender.Kind = "robot" }},
		{"missing sender id", func(e *Envelope) { e.Sender.ID = "" }},
		{"zero occurred at", func(e *Envelope) { e.OccurredAt = time.Time{} }},
		{"invalid payload", func(e *Envelope) { e.Payload = json.RawMessage(`{`) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}
