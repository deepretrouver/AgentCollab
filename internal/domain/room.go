package domain

import "time"

type RoomID string

type Room struct {
	ID          RoomID         `json:"id"`
	WorkspaceID WorkspaceID    `json:"workspace_id"`
	Name        string         `json:"name"`
	PlanStepID  *ProjectStepID `json:"plan_step_id,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	Pinned      bool           `json:"pinned"`
	PinnedAt    *time.Time     `json:"pinned_at,omitempty"`
}

type RoomMembership struct {
	RoomID   RoomID    `json:"room_id"`
	UserID   UserID    `json:"user_id"`
	JoinedAt time.Time `json:"joined_at"`
}

type RoomProjectStepUpdate struct {
	PlanStepID    *ProjectStepID `json:"plan_step_id,omitempty"`
	ClearPlanStep bool           `json:"clear_plan_step,omitempty"`
}
