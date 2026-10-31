package domain

import "time"

type WorkspaceID string

type WorkspaceRole string

const (
	WorkspaceOwner  WorkspaceRole = "owner"
	WorkspaceAdmin  WorkspaceRole = "admin"
	WorkspaceMember WorkspaceRole = "member"
)

type Workspace struct {
	ID          WorkspaceID   `json:"id"`
	Name        string        `json:"name"`
	OwnerUserID UserID        `json:"owner_user_id"`
	Role        WorkspaceRole `json:"role"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

type WorkspaceMembership struct {
	WorkspaceID WorkspaceID   `json:"workspace_id"`
	UserID      UserID        `json:"user_id"`
	Role        WorkspaceRole `json:"role"`
	JoinedAt    time.Time     `json:"joined_at"`
}
