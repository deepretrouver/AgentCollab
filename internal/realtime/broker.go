package realtime

import (
	"sync"

	"agentcollab.local/agentcollab/internal/events"
)

type Broker struct {
	mu         sync.Mutex
	bufferSize int
	rooms      map[string]map[*Subscription]chan events.Envelope
}

func NewBroker(bufferSize int) *Broker {
	if bufferSize < 1 {
		bufferSize = 1
	}
	return &Broker{bufferSize: bufferSize, rooms: make(map[string]map[*Subscription]chan events.Envelope)}
}

type Subscription struct {
	broker *Broker
	roomID string
	events chan events.Envelope
	once   sync.Once
}

func (b *Broker) Subscribe(roomID string) *Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	subscription := &Subscription{broker: b, roomID: roomID, events: make(chan events.Envelope, b.bufferSize)}
	if b.rooms[roomID] == nil {
		b.rooms[roomID] = make(map[*Subscription]chan events.Envelope)
	}
	b.rooms[roomID][subscription] = subscription.events
	return subscription
}

func (b *Broker) Publish(event events.Envelope) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subscriptions := b.rooms[event.RoomID]
	for subscription, channel := range subscriptions {
		select {
		case channel <- event:
		default:
			close(channel)
			delete(subscriptions, subscription)
		}
	}
	if len(subscriptions) == 0 {
		delete(b.rooms, event.RoomID)
	}
}

func (s *Subscription) Events() <-chan events.Envelope {
	return s.events
}

func (s *Subscription) Cancel() {
	s.once.Do(func() {
		s.broker.mu.Lock()
		defer s.broker.mu.Unlock()
		subscriptions := s.broker.rooms[s.roomID]
		if _, ok := subscriptions[s]; !ok {
			return
		}
		close(s.events)
		delete(subscriptions, s)
		if len(subscriptions) == 0 {
			delete(s.broker.rooms, s.roomID)
		}
	})
}
