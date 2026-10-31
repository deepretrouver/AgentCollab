package domain

import "time"

type WorkItemID string
type WorkItemKind string
type WorkItemStatus string
type WorkItemPriority string
type WorkItemMergeStatus string
type WorkItemAIReviewStatus string
type WorkItemVerificationStatus string

const (
	WorkItemTask       WorkItemKind = "task"
	WorkItemDiscussion WorkItemKind = "discussion"
	WorkItemBrainstorm WorkItemKind = "brainstorm"
)

const (
	WorkItemAIReviewNone             WorkItemAIReviewStatus = "none"
	WorkItemAIReviewPending          WorkItemAIReviewStatus = "pending"
	WorkItemAIReviewPassed           WorkItemAIReviewStatus = "passed"
	WorkItemAIReviewChangesRequested WorkItemAIReviewStatus = "changes_requested"
	WorkItemAIReviewFailed           WorkItemAIReviewStatus = "failed"
)

const (
	WorkItemVerificationNone    WorkItemVerificationStatus = "none"
	WorkItemVerificationPending WorkItemVerificationStatus = "pending"
	WorkItemVerificationPassed  WorkItemVerificationStatus = "passed"
	WorkItemVerificationFailed  WorkItemVerificationStatus = "failed"
)

const (
	WorkItemMergeNone     WorkItemMergeStatus = "none"
	WorkItemMergeOpen     WorkItemMergeStatus = "open"
	WorkItemMergeMerged   WorkItemMergeStatus = "merged"
	WorkItemMergeConflict WorkItemMergeStatus = "conflict"
)

const (
	WorkItemBacklog    WorkItemStatus = "backlog"
	WorkItemReady      WorkItemStatus = "ready"
	WorkItemInProgress WorkItemStatus = "in_progress"
	WorkItemReview     WorkItemStatus = "review"
	WorkItemDone       WorkItemStatus = "done"
	WorkItemCancelled  WorkItemStatus = "cancelled"
)

const (
	WorkItemPriorityLow    WorkItemPriority = "low"
	WorkItemPriorityNormal WorkItemPriority = "normal"
	WorkItemPriorityHigh   WorkItemPriority = "high"
	WorkItemPriorityUrgent WorkItemPriority = "urgent"
)

func (value WorkItemStatus) Valid() bool {
	switch value {
	case WorkItemBacklog, WorkItemReady, WorkItemInProgress, WorkItemReview, WorkItemDone, WorkItemCancelled:
		return true
	default:
		return false
	}
}

func (value WorkItemPriority) Valid() bool {
	switch value {
	case WorkItemPriorityLow, WorkItemPriorityNormal, WorkItemPriorityHigh, WorkItemPriorityUrgent:
		return true
	default:
		return false
	}
}

// WorkItem is the human-facing unit of work shown in a room's overview and
// board. Several Agent executions can contribute to the same item. Delivery
// therefore moves an item to review; only a human decision marks it done.
type WorkItem struct {
	ID                  WorkItemID                 `json:"id"`
	WorkspaceID         WorkspaceID                `json:"workspace_id"`
	RoomID              RoomID                     `json:"room_id"`
	RootMessageID       *MessageID                 `json:"root_message_id,omitempty"`
	PlanStepID          *ProjectStepID             `json:"plan_step_id,omitempty"`
	GitBranch           string                     `json:"git_branch,omitempty"`
	MergeStatus         WorkItemMergeStatus        `json:"merge_status"`
	MergedCommit        string                     `json:"merged_commit,omitempty"`
	BranchCommit        string                     `json:"branch_commit,omitempty"`
	ReviewAgentID       *AgentID                   `json:"review_agent_id,omitempty"`
	VerificationAgentID *AgentID                   `json:"verification_agent_id,omitempty"`
	ReviewTaskID        *TaskID                    `json:"review_task_id,omitempty"`
	VerificationTaskID  *TaskID                    `json:"verification_task_id,omitempty"`
	AIReviewStatus      WorkItemAIReviewStatus     `json:"ai_review_status"`
	AIReviewSummary     string                     `json:"ai_review_summary,omitempty"`
	VerificationStatus  WorkItemVerificationStatus `json:"verification_status"`
	VerificationSummary string                     `json:"verification_summary,omitempty"`
	Kind                WorkItemKind               `json:"kind"`
	Title               string                     `json:"title"`
	Description         string                     `json:"description"`
	Status              WorkItemStatus             `json:"status"`
	Priority            WorkItemPriority           `json:"priority"`
	OwnerUserID         *UserID                    `json:"owner_user_id,omitempty"`
	DueAt               *time.Time                 `json:"due_at,omitempty"`
	AcceptedAt          *time.Time                 `json:"accepted_at,omitempty"`
	TaskCount           int                        `json:"task_count"`
	ActiveTaskCount     int                        `json:"active_task_count"`
	DeliveredTaskCount  int                        `json:"delivered_task_count"`
	FailedTaskCount     int                        `json:"failed_task_count"`
	TargetAgentIDs      []AgentID                  `json:"target_agent_ids"`
	CreatedAt           time.Time                  `json:"created_at"`
	UpdatedAt           time.Time                  `json:"updated_at"`
}

type WorkItemUpdate struct {
	Title         *string           `json:"title,omitempty"`
	Status        *WorkItemStatus   `json:"status,omitempty"`
	Priority      *WorkItemPriority `json:"priority,omitempty"`
	OwnerUserID   *UserID           `json:"owner_user_id,omitempty"`
	ClearOwner    bool              `json:"clear_owner,omitempty"`
	DueAt         *time.Time        `json:"due_at,omitempty"`
	ClearDueAt    bool              `json:"clear_due_at,omitempty"`
	PlanStepID    *ProjectStepID    `json:"plan_step_id,omitempty"`
	ClearPlanStep bool              `json:"clear_plan_step,omitempty"`
}
