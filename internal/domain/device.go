package domain

import "time"

type DeviceID string
type RunnerID string

type DeviceStatus string

const (
	DeviceOnline  DeviceStatus = "online"
	DeviceOffline DeviceStatus = "offline"
	DeviceRevoked DeviceStatus = "revoked"
)

type Device struct {
	ID          DeviceID     `json:"id"`
	WorkspaceID WorkspaceID  `json:"workspace_id"`
	OwnerUserID UserID       `json:"owner_user_id"`
	Name        string       `json:"name"`
	StableKey   string       `json:"-"`
	OS          string       `json:"os"`
	Arch        string       `json:"arch"`
	Status      DeviceStatus `json:"status"`
	LastSeenAt  *time.Time   `json:"last_seen_at,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
}

type Runner struct {
	ID         RunnerID     `json:"id"`
	DeviceID   DeviceID     `json:"device_id"`
	TokenHash  string       `json:"-"`
	Version    string       `json:"version"`
	Status     DeviceStatus `json:"status"`
	LastSeenAt *time.Time   `json:"last_seen_at,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
}

type RunnerIdentity struct {
	RunnerID     RunnerID
	DeviceID     DeviceID
	WorkspaceID  WorkspaceID
	OwnerUserID  UserID
	DeviceStatus DeviceStatus
}

type RunnerEnrollment struct {
	Device Device          `json:"device"`
	Runner Runner          `json:"runner"`
	Agents []EnrolledAgent `json:"agents"`
}
