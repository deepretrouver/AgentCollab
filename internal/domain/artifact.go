package domain

import "time"

type ArtifactID string

type ArtifactStatus string

const (
	ArtifactPending ArtifactStatus = "pending"
	ArtifactReady   ArtifactStatus = "ready"
	ArtifactFailed  ArtifactStatus = "failed"
)

type Artifact struct {
	ID           ArtifactID
	WorkspaceID  WorkspaceID
	TaskID       TaskID
	AttemptNo    int
	Filename     string
	RelativePath string
	SizeBytes    int64
	SHA256       string
	Status       ArtifactStatus
	CreatedAt    time.Time
}
