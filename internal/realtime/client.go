package realtime

import (
	"context"
	"errors"
	"time"

	"github.com/gorilla/websocket"

	"agentcollab.local/agentcollab/internal/events"
)

var ErrSlowClient = errors.New("realtime client was dropped because its event buffer filled")

const (
	writeTimeout = 10 * time.Second
	pingInterval = 30 * time.Second
)

type Client struct {
	connection *websocket.Conn
}

func NewClient(connection *websocket.Conn) *Client {
	return &Client{connection: connection}
}

func (c *Client) Serve(ctx context.Context, replay []events.Envelope, live <-chan events.Envelope, after uint64) error {
	disconnected := make(chan struct{})
	go func() {
		defer close(disconnected)
		c.connection.SetReadLimit(1024)
		_ = c.connection.SetReadDeadline(time.Now().Add(2 * pingInterval))
		c.connection.SetPongHandler(func(string) error {
			return c.connection.SetReadDeadline(time.Now().Add(2 * pingInterval))
		})
		for {
			if _, _, err := c.connection.ReadMessage(); err != nil {
				return
			}
		}
	}()

	lastSequence := after
	for _, event := range replay {
		if event.Sequence <= lastSequence {
			continue
		}
		if err := c.writeJSON(event); err != nil {
			return err
		}
		lastSequence = event.Sequence
	}

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, open := <-live:
			if !open {
				return ErrSlowClient
			}
			if event.Sequence <= lastSequence {
				continue
			}
			if err := c.writeJSON(event); err != nil {
				return err
			}
			lastSequence = event.Sequence
		case <-ticker.C:
			if err := c.connection.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
				return err
			}
			if err := c.connection.WriteMessage(websocket.PingMessage, nil); err != nil {
				return err
			}
		case <-disconnected:
			return nil
		}
	}
}

func (c *Client) writeJSON(value any) error {
	if err := c.connection.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	return c.connection.WriteJSON(value)
}
