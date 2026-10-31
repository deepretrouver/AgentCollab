package domain

import "time"

type UserID string

type UserRole string
type RoomRole string

const (
	UserAdmin  UserRole = "admin"
	UserMember UserRole = "member"
)

const (
	RoomOwner  RoomRole = "owner"
	RoomAdmin  RoomRole = "admin"
	RoomMember RoomRole = "member"
)

type User struct {
	ID            UserID
	Username      string
	DisplayName   string
	AvatarURL     string
	Bio           string
	CurrentFocus  string
	PasswordHash  string
	Role          UserRole
	WorkspaceRole WorkspaceRole
	RoomRole      RoomRole
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
