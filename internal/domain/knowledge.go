package domain

import "time"

type KnowledgeAssetID string

type KnowledgeScope string

const (
	KnowledgeWorkspace KnowledgeScope = "workspace"
	KnowledgeRoom      KnowledgeScope = "room"
)

// KnowledgeAsset is a durable project reference file. Workspace assets are
// available to every room in the workspace; room assets stay with one room.
type KnowledgeAsset struct {
	ID             KnowledgeAssetID `json:"id"`
	WorkspaceID    WorkspaceID      `json:"workspace_id"`
	RoomID         *RoomID          `json:"room_id,omitempty"`
	Scope          KnowledgeScope   `json:"scope"`
	UploaderUserID UserID           `json:"uploader_user_id"`
	UploaderName   string           `json:"uploader_name,omitempty"`
	Filename       string           `json:"filename"`
	ContentType    string           `json:"content_type"`
	SizeBytes      int64            `json:"size_bytes"`
	SHA256         string           `json:"sha256"`
	RelativePath   string           `json:"-"`
	DownloadURL    string           `json:"download_url,omitempty"`
	CanDelete      bool             `json:"can_delete"`
	LocalPath      string           `json:"local_path,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
}

type ProjectFileScope struct {
	WorkspaceID WorkspaceID
	RoomID      *RoomID
}
