package domain

import (
	"encoding/json"
	"time"
)

type AgentID string

type AgentStatus string

type AgentResponseMode string
type AgentVisibility string
type AgentInvocationScope string

const (
	AgentOnline      AgentStatus = "online"
	AgentOffline     AgentStatus = "offline"
	AgentBusy        AgentStatus = "busy"
	AgentUnavailable AgentStatus = "unavailable"
)

const (
	AgentVisibilityPrivate     AgentVisibility      = "private"
	AgentVisibilityWorkspace   AgentVisibility      = "workspace"
	AgentInvocationOwnerOnly   AgentInvocationScope = "owner_only"
	AgentInvocationRoomMembers AgentInvocationScope = "room_members"
)

const (
	AgentResponseMention     AgentResponseMode = "mention"
	AgentResponseCoordinator AgentResponseMode = "coordinator"
	AgentResponseHumanAll    AgentResponseMode = "human_all"
	AgentResponseMuted       AgentResponseMode = "muted"
)

type Agent struct {
	ID           AgentID         `json:"id"`
	WorkspaceID  WorkspaceID     `json:"workspace_id"`
	RunnerID     RunnerID        `json:"runner_id"`
	OwnerUserID  UserID          `json:"owner_user_id"`
	Type         string          `json:"type"`
	DisplayName  string          `json:"display_name"`
	Status       AgentStatus     `json:"status"`
	Capabilities json.RawMessage `json:"capabilities"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type AgentIdentity struct {
	AgentID            AgentID              `json:"id"`
	DisplayName        string               `json:"display_name"`
	Type               string               `json:"type"`
	OwnerUserID        UserID               `json:"owner_user_id"`
	OwnerName          string               `json:"owner_name"`
	DeviceID           DeviceID             `json:"device_id"`
	DeviceName         string               `json:"device_name"`
	OS                 string               `json:"os"`
	Arch               string               `json:"arch"`
	Channel            string               `json:"channel"`
	Transport          string               `json:"transport"`
	CatalogTier        string               `json:"catalog_tier"`
	ProtocolVersion    int                  `json:"protocol_version,omitempty"`
	PermissionMode     string               `json:"permission_mode"`
	Status             AgentStatus          `json:"status"`
	Imported           bool                 `json:"imported"`
	Available          bool                 `json:"available"`
	Version            string               `json:"version"`
	RunnerVersion      string               `json:"runner_version"`
	RunnerLastSeenAt   *time.Time           `json:"runner_last_seen_at,omitempty"`
	QueuedTaskCount    int                  `json:"queued_task_count"`
	RunningTaskCount   int                  `json:"running_task_count"`
	NativeSessionReady bool                 `json:"native_session_ready"`
	Executable         string               `json:"executable"`
	NativeHistory      string               `json:"native_history"`
	Skills             []AgentSkill         `json:"skills"`
	Resources          []AgentResource      `json:"resources"`
	ResponseMode       AgentResponseMode    `json:"response_mode"`
	ReplyDelaySeconds  int                  `json:"reply_delay_seconds"`
	RolePrompt         string               `json:"role_prompt"`
	MaxAutoTurns       int                  `json:"max_auto_turns"`
	Visibility         AgentVisibility      `json:"visibility"`
	InvocationScope    AgentInvocationScope `json:"invocation_scope"`
	SharedSkills       []string             `json:"shared_skills"`
	CanManage          bool                 `json:"can_manage"`
	CanModerate        bool                 `json:"can_moderate"`
	CanInvoke          bool                 `json:"can_invoke"`
}

type AgentAccessPolicy struct {
	AgentID         AgentID              `json:"agent_id"`
	Visibility      AgentVisibility      `json:"visibility"`
	InvocationScope AgentInvocationScope `json:"invocation_scope"`
	SharedSkills    []string             `json:"shared_skills"`
}

type AgentSkill struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Source identifies the local Skill root that exposed this capability. It is
	// informational only; the Runner still decides whether a Skill can be read
	// and used on the target computer.
	Source string `json:"source,omitempty"`
}

type AgentResource struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Access      string `json:"access"`
}

type RoomAgentSettings struct {
	RoomID            RoomID            `json:"room_id"`
	AgentID           AgentID           `json:"agent_id"`
	ResponseMode      AgentResponseMode `json:"response_mode"`
	ReplyDelaySeconds int               `json:"reply_delay_seconds"`
	RolePrompt        string            `json:"role_prompt"`
	MaxAutoTurns      int               `json:"max_auto_turns"`
}

type AgentRegistration struct {
	LocalID      string          `json:"local_id"`
	Type         string          `json:"type"`
	DisplayName  string          `json:"display_name"`
	Capabilities json.RawMessage `json:"capabilities"`
	Available    bool            `json:"available"`
}

type ExecutionReceipt struct {
	NativeAgentType     string   `json:"native_agent_type"`
	Executable          string   `json:"executable"`
	Version             string   `json:"version"`
	PID                 int      `json:"pid"`
	StartedAt           string   `json:"started_at"`
	FinishedAt          string   `json:"finished_at"`
	NativeSessionID     string   `json:"native_session_id,omitempty"`
	NativeHistory       string   `json:"native_history"`
	SkillCount          int      `json:"skill_count,omitempty"`
	ResourceCount       int      `json:"resource_count,omitempty"`
	MatchedSkills       []string `json:"matched_skills,omitempty"`
	MatchedResources    []string `json:"matched_resources,omitempty"`
	ProjectStepID       string   `json:"project_step_id,omitempty"`
	ProjectCommit       string   `json:"project_commit,omitempty"`
	ExecutionSource     string   `json:"execution_source,omitempty"`
	LocalDirectoryLabel string   `json:"local_directory_label,omitempty"`
}

type EnrolledAgent struct {
	LocalID string  `json:"local_id"`
	AgentID AgentID `json:"agent_id"`
}
