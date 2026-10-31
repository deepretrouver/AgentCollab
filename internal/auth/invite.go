package auth

import (
	"context"
	"fmt"
	"time"

	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/idgen"
)

const maxInviteDuration = 7 * 24 * time.Hour

type Invite struct {
	ID              string
	WorkspaceID     domain.WorkspaceID
	RoomID          domain.RoomID
	CreatedByUserID domain.UserID
	CodeHash        string
	Role            domain.WorkspaceRole
	ExpiresAt       time.Time
	CreatedAt       time.Time
}

type InviteResult struct {
	Invite Invite
	Code   string
}

func (s *Service) CreateInvite(ctx context.Context, actorID domain.UserID, workspaceID domain.WorkspaceID, role domain.WorkspaceRole, duration time.Duration, roomIDs ...domain.RoomID) (InviteResult, error) {
	if role != domain.WorkspaceAdmin && role != domain.WorkspaceMember {
		return InviteResult{}, fmt.Errorf("%w: invite role must be admin or member", ErrInvalidInput)
	}
	if duration <= 0 || duration > maxInviteDuration {
		return InviteResult{}, fmt.Errorf("%w: invite duration must be between one second and seven days", ErrInvalidInput)
	}
	if len(roomIDs) > 1 {
		return InviteResult{}, fmt.Errorf("%w: only one room may be attached to an invite", ErrInvalidInput)
	}
	var roomID domain.RoomID
	if len(roomIDs) == 1 {
		roomID = roomIDs[0]
	}
	actorRole, err := s.repository.WorkspaceRole(ctx, workspaceID, actorID)
	authorized := err == nil && (actorRole == domain.WorkspaceOwner || actorRole == domain.WorkspaceAdmin)
	if roomID != "" {
		if role != domain.WorkspaceMember {
			return InviteResult{}, fmt.Errorf("%w: room invites always add a regular workspace member", ErrInvalidInput)
		}
		if !authorized {
			roomRole, roomErr := s.repository.RoomRole(ctx, roomID, actorID)
			authorized = roomErr == nil && (roomRole == domain.RoomOwner || roomRole == domain.RoomAdmin)
		}
	}
	if !authorized {
		return InviteResult{}, ErrForbidden
	}
	code, err := newRawToken(24)
	if err != nil {
		return InviteResult{}, err
	}
	id, err := idgen.New("invite")
	if err != nil {
		return InviteResult{}, err
	}
	now := s.now()
	invite := Invite{
		ID:              id,
		WorkspaceID:     workspaceID,
		RoomID:          roomID,
		CreatedByUserID: actorID,
		CodeHash:        HashToken(code),
		Role:            role,
		ExpiresAt:       now.Add(duration),
		CreatedAt:       now,
	}
	if err := s.repository.CreateInvite(ctx, invite); err != nil {
		return InviteResult{}, err
	}
	return InviteResult{Invite: invite, Code: code}, nil
}

func (s *Service) AcceptInvite(ctx context.Context, code string, userID domain.UserID) (domain.WorkspaceMembership, error) {
	if code == "" {
		return domain.WorkspaceMembership{}, ErrInviteInvalid
	}
	return s.repository.ConsumeInvite(ctx, HashToken(code), userID, s.now())
}
