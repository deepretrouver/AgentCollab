package domain

import (
	"fmt"
	"time"
)

type TaskID string
type TaskAttemptID string

type TaskCreatorKind string

type TaskExecutionSource string

const (
	TaskCreatorUser  TaskCreatorKind = "user"
	TaskCreatorAgent TaskCreatorKind = "agent"
)

const (
	// Empty is retained for existing tasks and means the canonical Server
	// snapshot when a project step is attached.
	TaskExecutionOfficial         TaskExecutionSource = "official"
	TaskExecutionResponsibleLocal TaskExecutionSource = "responsible_local"
	TaskExecutionNone             TaskExecutionSource = "none"
)

func (value TaskExecutionSource) Valid() bool {
	return value == "" || value == TaskExecutionOfficial || value == TaskExecutionResponsibleLocal || value == TaskExecutionNone
}

type TaskState string

const (
	TaskQueued     TaskState = "queued"
	TaskDispatched TaskState = "dispatched"
	TaskRunning    TaskState = "running"
	TaskDelivered  TaskState = "delivered"
	TaskFailed     TaskState = "failed"
	TaskBlocked    TaskState = "blocked"
	TaskCancelled  TaskState = "cancelled"
)

var taskTransitions = map[TaskState]map[TaskState]struct{}{
	TaskQueued: {
		TaskDispatched: {},
		TaskBlocked:    {},
		TaskCancelled:  {},
	},
	TaskDispatched: {
		TaskRunning:   {},
		TaskBlocked:   {},
		TaskCancelled: {},
	},
	TaskRunning: {
		TaskDelivered: {},
		TaskFailed:    {},
		TaskBlocked:   {},
		TaskCancelled: {},
	},
	TaskBlocked: {
		TaskQueued:    {},
		TaskCancelled: {},
	},
	TaskFailed: {
		TaskQueued:    {},
		TaskCancelled: {},
	},
}

func (s TaskState) Valid() bool {
	switch s {
	case TaskQueued, TaskDispatched, TaskRunning, TaskDelivered, TaskFailed, TaskBlocked, TaskCancelled:
		return true
	default:
		return false
	}
}

func (s TaskState) CanTransitionTo(next TaskState) bool {
	_, ok := taskTransitions[s][next]
	return ok
}

func (s TaskState) Terminal() bool {
	return s == TaskDelivered || s == TaskCancelled
}

func ValidateTaskTransition(from, to TaskState) error {
	if !from.Valid() || !to.Valid() {
		return fmt.Errorf("invalid task state transition %q -> %q", from, to)
	}
	if !from.CanTransitionTo(to) {
		return fmt.Errorf("task state transition %q -> %q is not allowed", from, to)
	}
	return nil
}

type Task struct {
	ID                 TaskID              `json:"id"`
	WorkItemID         *WorkItemID         `json:"work_item_id,omitempty"`
	PlanStepID         *ProjectStepID      `json:"plan_step_id,omitempty"`
	ExecutionSource    TaskExecutionSource `json:"execution_source,omitempty"`
	WorkspaceID        WorkspaceID         `json:"workspace_id"`
	RoomID             RoomID              `json:"room_id"`
	ParentTaskID       *TaskID             `json:"parent_task_id,omitempty"`
	RootMessageID      *MessageID          `json:"root_message_id,omitempty"`
	SourceMessageID    *MessageID          `json:"source_message_id,omitempty"`
	CollaborationDepth int                 `json:"collaboration_depth"`
	CreatorKind        TaskCreatorKind     `json:"creator_kind"`
	CreatorID          string              `json:"creator_id"`
	TargetAgentID      AgentID             `json:"target_agent_id"`
	Title              string              `json:"title"`
	Prompt             string              `json:"prompt"`
	AcceptanceCriteria string              `json:"acceptance_criteria"`
	State              TaskState           `json:"state"`
	CurrentAttempt     int                 `json:"current_attempt"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

type TaskAttempt struct {
	ID           TaskAttemptID `json:"id"`
	TaskID       TaskID        `json:"task_id"`
	AttemptNo    int           `json:"attempt_no"`
	State        TaskState     `json:"state"`
	RunnerID     *RunnerID     `json:"runner_id,omitempty"`
	StartedAt    *time.Time    `json:"started_at,omitempty"`
	FinishedAt   *time.Time    `json:"finished_at,omitempty"`
	ExitCode     *int          `json:"exit_code,omitempty"`
	ErrorSummary string        `json:"error_summary,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
}

// ProjectContextSnapshot is the compact, durable shared context attached by
// the Hub when a task belongs to a project step. It is intentionally factual:
// plan and step fields come from the Server database, while documents remain
// separate KnowledgeAssets with their own provenance.
type ProjectContextSnapshot struct {
	Objective       string            `json:"objective,omitempty"`
	StepID          ProjectStepID     `json:"step_id,omitempty"`
	StepTitle       string            `json:"step_title,omitempty"`
	StepDescription string            `json:"step_description,omitempty"`
	StepStatus      ProjectStepStatus `json:"step_status,omitempty"`
}

// CollaborationEnvelope is the stable routing contract between Hub and
// Runner. Its fields are derived from persisted task/message identifiers, so a
// retry keeps the same envelope ID and consumers can process it idempotently.
type CollaborationEnvelope struct {
	ID            TaskID     `json:"id"`
	Conversation  string     `json:"conversation"`
	InReplyTo     *MessageID `json:"in_reply_to,omitempty"`
	From          string     `json:"from"`
	To            AgentID    `json:"to"`
	Act           string     `json:"act"`
	Hops          int        `json:"hops"`
	RequiresReply bool       `json:"requires_reply"`
	NeedsHuman    bool       `json:"needs_human"`
}

func (task Task) Envelope() CollaborationEnvelope {
	conversation := string(task.ID)
	if task.RootMessageID != nil {
		conversation = string(*task.RootMessageID)
	}
	return CollaborationEnvelope{
		ID: task.ID, Conversation: conversation, InReplyTo: task.SourceMessageID,
		From: task.CreatorID, To: task.TargetAgentID, Act: "request",
		Hops: task.CollaborationDepth, RequiresReply: true,
	}
}

type ClaimedTask struct {
	Task
	AttemptNo int                   `json:"attempt_no"`
	Envelope  CollaborationEnvelope `json:"envelope"`
	// TargetAgentSourceID is the original server Agent registration held by
	// the Runner. A workspace can expose a separate Agent record while the
	// Runner credential remains keyed by this canonical registration.
	TargetAgentSourceID AgentID `json:"target_agent_source_id,omitempty"`
	// TargetAgentLocalKey is retained as a final fallback for current Runner
	// catalogs, including new registrations that have no canonical source yet.
	TargetAgentLocalKey string `json:"target_agent_local_key,omitempty"`
	// TargetAgentType lets a Runner recover after its historical server Agent
	// ID and local key were both replaced by a workspace migration. It is only
	// used when exactly one available local configuration has this type.
	TargetAgentType   string                    `json:"target_agent_type,omitempty"`
	NativeSessionID   string                    `json:"native_session_id,omitempty"`
	Attachments       []Attachment              `json:"attachments,omitempty"`
	KnowledgeAssets   []KnowledgeAsset          `json:"knowledge_assets,omitempty"`
	ProjectContext    *ProjectContextSnapshot   `json:"project_context,omitempty"`
	ProjectWorkspace  *ProjectWorkspaceSnapshot `json:"project_workspace,omitempty"`
	LocalWorkspace    *LocalProjectWorkspace    `json:"local_workspace,omitempty"`
	DeliveryDirectory string                    `json:"-"`
	AuthorizedSkills  []string                  `json:"authorized_skills,omitempty"`
	CapabilityScope   string                    `json:"capability_scope,omitempty"`
}
