package realtime

import (
	"testing"
	"time"

	"agentcollab.local/agentcollab/internal/events"
)

func TestBrokerPublishesByRoom(t *testing.T) {
	broker := NewBroker(4)
	first := broker.Subscribe("room-1")
	defer first.Cancel()
	second := broker.Subscribe("room-2")
	defer second.Cancel()

	want := events.Envelope{RoomID: "room-1", Sequence: 1}
	broker.Publish(want)
	select {
	case got := <-first.Events():
		if got.RoomID != want.RoomID || got.Sequence != want.Sequence {
			t.Fatalf("event = %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("room-1 subscriber did not receive event")
	}
	select {
	case got := <-second.Events():
		t.Fatalf("room-2 received unexpected event %#v", got)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestBrokerDropsSlowSubscriberWithoutBlocking(t *testing.T) {
	broker := NewBroker(1)
	slow := broker.Subscribe("room-1")
	broker.Publish(events.Envelope{RoomID: "room-1", Sequence: 1})
	broker.Publish(events.Envelope{RoomID: "room-1", Sequence: 2})

	if got := <-slow.Events(); got.Sequence != 1 {
		t.Fatalf("first buffered event sequence = %d", got.Sequence)
	}
	if _, open := <-slow.Events(); open {
		t.Fatal("slow subscription remains open")
	}
}
