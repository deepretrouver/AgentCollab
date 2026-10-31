package auth

import (
	"context"
	"fmt"
	"time"

	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/idgen"
)

const maxRunnerInviteDuration = 24 * time.Hour

// RunnerInvite is a one-time grant for enrolling one remote Runner. The raw
// code is returned once; only its hash is persisted by the Hub.
type RunnerInvite struct {
	ID              string
	WorkspaceID     domain.WorkspaceID
	OwnerUserID     domain.UserID
	CreatedByUserID domain.UserID
	CodeHash        string
	ExpiresAt       time.Time
	CreatedAt       time.Time
}

type RunnerInviteResult struct {
	Invite RunnerInvite
	Code   string
}

func (s *Service) CreateRunnerInvite(ctx context.Context, actorID domain.UserID, workspaceID domain.WorkspaceID, duration time.Duration) (RunnerInviteResult, error) {
	role, err := s.repository.WorkspaceRole(ctx, workspaceID, actorID)
	if err != nil || (role != domain.WorkspaceOwner && role != domain.WorkspaceAdmin && role != domain.WorkspaceMember) {
		return RunnerInviteResult{}, ErrForbidden
	}
	if duration <= 0 || duration > maxRunnerInviteDuration {
		return RunnerInviteResult{}, fmt.Errorf("%w: runner invite duration must be between one second and 24 hours", ErrInvalidInput)
	}
	code, err := newRawToken(24)
	if err != nil {
		return RunnerInviteResult{}, err
	}
	id, err := idgen.New("runinvite")
	if err != nil {
		return RunnerInviteResult{}, err
	}
	now := s.now()
	invite := RunnerInvite{
		ID:              id,
		WorkspaceID:     workspaceID,
		OwnerUserID:     actorID,
		CreatedByUserID: actorID,
		CodeHash:        HashToken(code),
		ExpiresAt:       now.Add(duration),
		CreatedAt:       now,
	}
	if err := s.repository.CreateRunnerInvite(ctx, invite); err != nil {
		return RunnerInviteResult{}, err
	}
	return RunnerInviteResult{Invite: invite, Code: code}, nil
}
