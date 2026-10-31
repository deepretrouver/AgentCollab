package domain

import "time"

type ProjectStepID string
type ProjectPlanStatus string
type ProjectStepStatus string

const (
	ProjectPlanPlanning  ProjectPlanStatus = "planning"
	ProjectPlanActive    ProjectPlanStatus = "active"
	ProjectPlanCompleted ProjectPlanStatus = "completed"
	ProjectPlanCancelled ProjectPlanStatus = "cancelled"
)

const (
	ProjectStepPlanned    ProjectStepStatus = "planned"
	ProjectStepInProgress ProjectStepStatus = "in_progress"
	ProjectStepReview     ProjectStepStatus = "review"
	ProjectStepDone       ProjectStepStatus = "done"
	ProjectStepCancelled  ProjectStepStatus = "cancelled"
)

func (value ProjectPlanStatus) Valid() bool {
	switch value {
	case ProjectPlanPlanning, ProjectPlanActive, ProjectPlanCompleted, ProjectPlanCancelled:
		return true
	default:
		return false
	}
}

func (value ProjectStepStatus) Valid() bool {
	switch value {
	case ProjectStepPlanned, ProjectStepInProgress, ProjectStepReview, ProjectStepDone, ProjectStepCancelled:
		return true
	default:
		return false
	}
}

// ProjectPlan is the workspace-level project definition. Progress is derived only
// from accepted plan steps; chat volume and Agent execution counts do not
// affect it.
type ProjectPlan struct {
	WorkspaceID WorkspaceID       `json:"workspace_id"`
	Objective   string            `json:"objective"`
	Status      ProjectPlanStatus `json:"status"`
	Progress    int               `json:"progress"`
	CanManage   bool              `json:"can_manage"`
	Steps       []ProjectStep     `json:"steps"`
	CreatedAt   *time.Time        `json:"created_at,omitempty"`
	UpdatedAt   *time.Time        `json:"updated_at,omitempty"`
}

type ProjectStep struct {
	ID                  ProjectStepID             `json:"id"`
	WorkspaceID         WorkspaceID               `json:"workspace_id"`
	ParentStepID        *ProjectStepID            `json:"parent_step_id,omitempty"`
	Title               string                    `json:"title"`
	Description         string                    `json:"description"`
	Weight              int                       `json:"weight"`
	Status              ProjectStepStatus         `json:"status"`
	SortOrder           int                       `json:"sort_order"`
	Progress            int                       `json:"progress"`
	LinkedItemCount     int                       `json:"linked_item_count"`
	ResponsibleUserIDs  []UserID                  `json:"responsible_user_ids"`
	ResponsibleAgentIDs []AgentID                 `json:"responsible_agent_ids"`
	Workspace           *ProjectStepWorkspace     `json:"workspace,omitempty"`
	LocalBindings       []ProjectStepLocalBinding `json:"local_bindings"`
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
}

// ProjectStepLocalBinding is the collaborative record that a responsible
// member has selected a live working directory on one of their devices. The
// absolute path never leaves that member's Client; the Server stores only the
// label and device/Runner relationship needed for safe task routing.
type ProjectStepLocalBinding struct {
	StepID         ProjectStepID `json:"step_id"`
	UserID         UserID        `json:"user_id"`
	RunnerID       RunnerID      `json:"runner_id"`
	DeviceID       DeviceID      `json:"device_id"`
	DeviceName     string        `json:"device_name"`
	DirectoryLabel string        `json:"directory_label"`
	BaselineCommit string        `json:"baseline_commit,omitempty"`
	RunnerStatus   DeviceStatus  `json:"runner_status"`
	LastSeenAt     *time.Time    `json:"last_seen_at,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type ProjectStepLocalBindingUpdate struct {
	RunnerID       RunnerID `json:"runner_id"`
	DeviceID       DeviceID `json:"device_id"`
	DirectoryLabel string   `json:"directory_label"`
}

// LocalProjectWorkspace tells the target Runner to use its owner's persistent
// directory for this step. LocalPath is populated only inside that Runner.
type LocalProjectWorkspace struct {
	StepID         ProjectStepID `json:"step_id"`
	DirectoryLabel string        `json:"directory_label"`
	BaselineCommit string        `json:"baseline_commit,omitempty"`
	LocalPath      string        `json:"local_path,omitempty"`
}

// ProjectStepWorkspace describes the canonical directory snapshot for a plan
// step. The actual files live inside the protected workspace Git repository.
type ProjectStepWorkspace struct {
	StepID          ProjectStepID `json:"step_id"`
	SourceName      string        `json:"source_name"`
	FileCount       int           `json:"file_count"`
	TotalBytes      int64         `json:"total_bytes"`
	Revision        int           `json:"revision"`
	LastCommit      string        `json:"last_commit"`
	UpdatedByUserID *UserID       `json:"updated_by_user_id,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// ProjectWorkspaceSnapshot is the immutable project-step tree made available
// to a Runner for one task. DownloadURL is Runner-authenticated; LocalPath is
// populated only after the Client has downloaded and verified the archive.
type ProjectWorkspaceSnapshot struct {
	WorkspaceID WorkspaceID   `json:"workspace_id"`
	StepID      ProjectStepID `json:"step_id"`
	SourceName  string        `json:"source_name"`
	Revision    int           `json:"revision"`
	Commit      string        `json:"commit"`
	FileCount   int           `json:"file_count"`
	TotalBytes  int64         `json:"total_bytes"`
	DownloadURL string        `json:"download_url"`
	LocalPath   string        `json:"local_path,omitempty"`
}

type ProjectPlanUpdate struct {
	Objective *string            `json:"objective,omitempty"`
	Status    *ProjectPlanStatus `json:"status,omitempty"`
}

type ProjectStepCreate struct {
	ParentStepID        *ProjectStepID    `json:"parent_step_id,omitempty"`
	Title               string            `json:"title"`
	Description         string            `json:"description"`
	Weight              int               `json:"weight"`
	Status              ProjectStepStatus `json:"status"`
	SortOrder           int               `json:"sort_order"`
	ResponsibleUserIDs  []UserID          `json:"responsible_user_ids,omitempty"`
	ResponsibleAgentIDs []AgentID         `json:"responsible_agent_ids,omitempty"`
}

type ProjectStepUpdate struct {
	ParentStepID        *ProjectStepID     `json:"parent_step_id,omitempty"`
	ClearParent         bool               `json:"clear_parent,omitempty"`
	Title               *string            `json:"title,omitempty"`
	Description         *string            `json:"description,omitempty"`
	Weight              *int               `json:"weight,omitempty"`
	Status              *ProjectStepStatus `json:"status,omitempty"`
	SortOrder           *int               `json:"sort_order,omitempty"`
	ResponsibleUserIDs  *[]UserID          `json:"responsible_user_ids,omitempty"`
	ResponsibleAgentIDs *[]AgentID         `json:"responsible_agent_ids,omitempty"`
}
