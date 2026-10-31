package realtime

import (
	"context"
	"fmt"

	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/events"
)

const (
	replayBatchSize = 200
	maxReplayEvents = 5000
)

type Repository interface {
	RoomWorkspace(context.Context, domain.RoomID, domain.UserID) (domain.WorkspaceID, error)
	ReplayRoomEvents(context.Context, domain.RoomID, domain.UserID, uint64, int) ([]events.Envelope, error)
}

func LoadReplay(ctx context.Context, repository Repository, roomID domain.RoomID, userID domain.UserID, after uint64) ([]events.Envelope, error) {
	result := make([]events.Envelope, 0)
	lastSequence := after
	for {
		batch, err := repository.ReplayRoomEvents(ctx, roomID, userID, lastSequence, replayBatchSize)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			return result, nil
		}
		result = append(result, batch...)
		lastSequence = batch[len(batch)-1].Sequence
		if len(result) > maxReplayEvents {
			return nil, fmt.Errorf("replay exceeds %d events; resync through the timeline API", maxReplayEvents)
		}
		if len(batch) < replayBatchSize {
			return result, nil
		}
	}
}
