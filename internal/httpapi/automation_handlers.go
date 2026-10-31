package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/events"
	"agentcollab.local/agentcollab/internal/idgen"
	"agentcollab.local/agentcollab/internal/storage"
)

type AutomationRepository interface {
	EnrollRunner(context.Context, domain.UserID, domain.WorkspaceID, string, string, string, string, string, string, []domain.AgentRegistration) (domain.RunnerEnrollment, error)
	EnrollRunnerWithInvite(context.Context, string, string, string, string, string, string, string, []domain.AgentRegistration) (domain.RunnerEnrollment, error)
	AuthenticateRunner(context.Context, string) (domain.RunnerIdentity, error)
	RunnerOwnedByUser(context.Context, domain.RunnerID, domain.UserID) (bool, error)
	HeartbeatRunner(context.Context, domain.RunnerID, string, string, []domain.AgentID, []string, []domain.AgentRegistration) ([]domain.EnrolledAgent, error)
	ListAgents(context.Context, domain.WorkspaceID, domain.UserID) ([]domain.AgentIdentity, error)
	ListAgentCandidates(context.Context, domain.WorkspaceID, domain.UserID) ([]domain.AgentIdentity, error)
	ImportAgent(context.Context, domain.WorkspaceID, domain.AgentID, domain.UserID) error
	RemoveAgent(context.Context, domain.WorkspaceID, domain.AgentID, domain.UserID) error
	ListRoomAgents(context.Context, domain.RoomID, domain.UserID) ([]domain.AgentIdentity, error)
	AddRoomAgent(context.Context, domain.RoomID, domain.AgentID, domain.UserID) error
	RemoveRoomAgent(context.Context, domain.RoomID, domain.AgentID, domain.UserID) error
	UpdateRoomAgentSettings(context.Context, domain.RoomAgentSettings, domain.UserID) error
	UpdateAgentAccessPolicy(context.Context, domain.WorkspaceID, domain.AgentAccessPolicy, domain.UserID) error
	CreateTask(context.Context, domain.Task, domain.UserID) (domain.Task, domain.Message, error)
	ListRoomTasks(context.Context, domain.RoomID, domain.UserID) ([]domain.Task, error)
	CancelTasksForRoot(context.Context, domain.RoomID, domain.MessageID, domain.UserID) ([]domain.Task, domain.Message, error)
	RecoverInterruptedTasks(context.Context, domain.RunnerID) (int, error)
	ClaimNextTask(context.Context, domain.RunnerID) (*domain.ClaimedTask, error)
	StartTask(context.Context, domain.RunnerID, domain.TaskID) error
	CompleteTask(context.Context, domain.RunnerID, domain.TaskID, bool, string, string, int, domain.ExecutionReceipt) (domain.Message, error)
	TaskCancellationRequested(context.Context, domain.RunnerID, domain.TaskID) (bool, error)
	ProjectWorkspaceForRunnerTask(context.Context, domain.TaskID, domain.RunnerID) (domain.ProjectWorkspaceSnapshot, error)
}

type AutomationHandlers struct {
	auth        *auth.Service
	repository  AutomationRepository
	publisher   EventPublisher
	storageRoot string
}

func NewAutomationHandlers(authService *auth.Service, repository AutomationRepository, publisher EventPublisher, storageRoots ...string) *AutomationHandlers {
	handler := &AutomationHandlers{auth: authService, repository: repository, publisher: publisher}
	if len(storageRoots) > 0 {
		handler.storageRoot = storageRoots[0]
	}
	return handler
}

func (h *AutomationHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/runner/enroll", h.enroll)
	mux.HandleFunc("POST /api/v1/runner/heartbeat", h.heartbeat)
	mux.HandleFunc("GET /api/v1/runners/{runnerID}/ownership", h.runnerOwnership)
	mux.HandleFunc("POST /api/v1/runner/tasks/recover", h.recoverTasks)
	mux.HandleFunc("GET /api/v1/runner/tasks/next", h.nextTask)
	mux.HandleFunc("POST /api/v1/runner/tasks/{taskID}/start", h.startTask)
	mux.HandleFunc("POST /api/v1/runner/tasks/{taskID}/complete", h.completeTask)
	mux.HandleFunc("GET /api/v1/runner/tasks/{taskID}/cancellation", h.taskCancellation)
	mux.HandleFunc("GET /api/v1/runner/tasks/{taskID}/project-workspace", h.downloadProjectWorkspace)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/agents", h.listAgents)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/agent-candidates", h.listAgentCandidates)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/agents/import", h.importAgent)
	mux.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/agents/{agentID}", h.removeAgent)
	mux.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}/agents/{agentID}/policy", h.updateAgentPolicy)
	mux.HandleFunc("GET /api/v1/rooms/{roomID}/agents", h.listRoomAgents)
	mux.HandleFunc("POST /api/v1/rooms/{roomID}/agents", h.addRoomAgent)
	mux.HandleFunc("DELETE /api/v1/rooms/{roomID}/agents/{agentID}", h.removeRoomAgent)
	mux.HandleFunc("PATCH /api/v1/rooms/{roomID}/agents/{agentID}/settings", h.updateRoomAgentSettings)
	mux.HandleFunc("GET /api/v1/rooms/{roomID}/tasks", h.listRoomTasks)
	mux.HandleFunc("POST /api/v1/rooms/{roomID}/tasks", h.createTask)
	mux.HandleFunc("POST /api/v1/rooms/{roomID}/task-roots/{rootMessageID}/cancel", h.cancelTaskRoot)
}

func (h *AutomationHandlers) downloadProjectWorkspace(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authenticateRunner(w, r)
	if !ok {
		return
	}
	if h.storageRoot == "" {
		writeError(w, http.StatusServiceUnavailable, "repository_unavailable", "Server 尚未配置项目存储目录")
		return
	}
	snapshot, err := h.repository.ProjectWorkspaceForRunnerTask(r.Context(), domain.TaskID(r.PathValue("taskID")), identity.RunnerID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	archive, err := storage.CreateProjectStepArchive(h.storageRoot, string(snapshot.WorkspaceID), string(snapshot.StepID), snapshot.Commit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "project_workspace_unavailable", "无法生成任务项目工作区")
		return
	}
	defer archive.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="agentcollab-project.zip"`)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", archive.Size))
	w.Header().Set("X-AgentCollab-Project-Commit", snapshot.Commit)
	w.Header().Set("X-AgentCollab-Project-Step", string(snapshot.StepID))
	http.ServeContent(w, r, "agentcollab-project.zip", time.Time{}, archive)
}

func (h *AutomationHandlers) runnerOwnership(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	owned, err := h.repository.RunnerOwnedByUser(r.Context(), domain.RunnerID(r.PathValue("runnerID")), user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"owned_by_current_user": owned})
}

func (h *AutomationHandlers) updateAgentPolicy(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	var input struct {
		Visibility      domain.AgentVisibility      `json:"visibility"`
		InvocationScope domain.AgentInvocationScope `json:"invocation_scope"`
		SharedSkills    []string                    `json:"shared_skills"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if (input.Visibility != domain.AgentVisibilityPrivate && input.Visibility != domain.AgentVisibilityWorkspace) ||
		(input.InvocationScope != domain.AgentInvocationOwnerOnly && input.InvocationScope != domain.AgentInvocationRoomMembers) || len(input.SharedSkills) > 500 {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid Agent access policy")
		return
	}
	policy := domain.AgentAccessPolicy{AgentID: domain.AgentID(r.PathValue("agentID")), Visibility: input.Visibility, InvocationScope: input.InvocationScope, SharedSkills: input.SharedSkills}
	if err := h.repository.UpdateAgentAccessPolicy(r.Context(), domain.WorkspaceID(r.PathValue("workspaceID")), policy, user.ID); err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AutomationHandlers) enroll(w http.ResponseWriter, r *http.Request) {
	var input struct {
		EnrollmentCode string                     `json:"enrollment_code"`
		WorkspaceID    string                     `json:"workspace_id"`
		DeviceName     string                     `json:"device_name"`
		DeviceKey      string                     `json:"device_key"`
		OS             string                     `json:"os"`
		Arch           string                     `json:"arch"`
		Version        string                     `json:"version"`
		Agents         []domain.AgentRegistration `json:"agents"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	input.DeviceName = strings.TrimSpace(input.DeviceName)
	input.DeviceKey = strings.TrimSpace(input.DeviceKey)
	input.EnrollmentCode = strings.TrimSpace(input.EnrollmentCode)
	if (input.EnrollmentCode == "" && input.WorkspaceID == "") || input.DeviceName == "" || len(input.Agents) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "enrollment_code or workspace_id, device_name, and agents are required")
		return
	}
	if input.DeviceKey != "" && (len(input.DeviceKey) < 16 || len(input.DeviceKey) > 128) {
		writeError(w, http.StatusBadRequest, "invalid_request", "device_key must be 16-128 characters")
		return
	}
	if input.OS == "" {
		input.OS = runtime.GOOS
	}
	if input.Arch == "" {
		input.Arch = runtime.GOARCH
	}
	for index := range input.Agents {
		input.Agents[index].LocalID = strings.TrimSpace(input.Agents[index].LocalID)
		input.Agents[index].Type = strings.TrimSpace(input.Agents[index].Type)
		input.Agents[index].DisplayName = strings.TrimSpace(input.Agents[index].DisplayName)
		if input.Agents[index].LocalID == "" || input.Agents[index].Type == "" || input.Agents[index].DisplayName == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "each agent requires local_id, type, and display_name")
			return
		}
	}
	token, err := auth.NewDeviceToken()
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	var enrollment domain.RunnerEnrollment
	if input.EnrollmentCode != "" {
		enrollment, err = h.repository.EnrollRunnerWithInvite(r.Context(), auth.HashToken(input.EnrollmentCode), auth.HashToken(token), input.DeviceName, input.DeviceKey, input.OS, input.Arch, input.Version, input.Agents)
	} else {
		user, ok := h.authenticateUser(w, r)
		if !ok {
			return
		}
		enrollment, err = h.repository.EnrollRunner(r.Context(), user.ID, domain.WorkspaceID(input.WorkspaceID), auth.HashToken(token), input.DeviceName, input.DeviceKey, input.OS, input.Arch, input.Version, input.Agents)
	}
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"enrollment": enrollment, "token": token})
}

func (h *AutomationHandlers) heartbeat(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authenticateRunner(w, r)
	if !ok {
		return
	}
	var input struct {
		Version    string                     `json:"version"`
		DeviceKey  string                     `json:"device_key"`
		AgentIDs   []domain.AgentID           `json:"agent_ids"`
		AgentTypes []string                   `json:"agent_types"`
		Agents     []domain.AgentRegistration `json:"agents"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	input.DeviceKey = strings.TrimSpace(input.DeviceKey)
	if input.DeviceKey != "" && (len(input.DeviceKey) < 16 || len(input.DeviceKey) > 128) {
		writeError(w, http.StatusBadRequest, "invalid_request", "device_key must be 16-128 characters")
		return
	}
	if len(input.Agents) > 500 {
		writeError(w, http.StatusBadRequest, "invalid_request", "too many Agent registrations")
		return
	}
	for index := range input.Agents {
		input.Agents[index].LocalID = strings.TrimSpace(input.Agents[index].LocalID)
		input.Agents[index].Type = strings.TrimSpace(input.Agents[index].Type)
		input.Agents[index].DisplayName = strings.TrimSpace(input.Agents[index].DisplayName)
		if input.Agents[index].LocalID == "" || input.Agents[index].Type == "" || input.Agents[index].DisplayName == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "each agent requires local_id, type, and display_name")
			return
		}
	}
	agents, err := h.repository.HeartbeatRunner(r.Context(), identity.RunnerID, input.Version, input.DeviceKey, input.AgentIDs, input.AgentTypes, input.Agents)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	// Preserve compatibility with older Clients and integrations that only use
	// a heartbeat as an online signal. New Clients send their local catalog and
	// receive stable server IDs so newly installed ACP adapters need no re-pair.
	if len(input.Agents) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// recoverTasks is called once as a Runner process starts. Any task still
// marked active for this same Runner belongs to an older process that was
// stopped before it could report completion, so it is safe to make it
// claimable again rather than leaving the conversation permanently stuck.
func (h *AutomationHandlers) recoverTasks(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authenticateRunner(w, r)
	if !ok {
		return
	}
	requeued, err := h.repository.RecoverInterruptedTasks(r.Context(), identity.RunnerID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"requeued": requeued})
}

func (h *AutomationHandlers) nextTask(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authenticateRunner(w, r)
	if !ok {
		return
	}
	task, err := h.repository.ClaimNextTask(r.Context(), identity.RunnerID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	if task == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *AutomationHandlers) startTask(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authenticateRunner(w, r)
	if !ok {
		return
	}
	if err := h.repository.StartTask(r.Context(), identity.RunnerID, domain.TaskID(r.PathValue("taskID"))); err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AutomationHandlers) completeTask(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authenticateRunner(w, r)
	if !ok {
		return
	}
	var input struct {
		Success      bool                    `json:"success"`
		Output       string                  `json:"output"`
		ErrorSummary string                  `json:"error_summary"`
		ExitCode     int                     `json:"exit_code"`
		Receipt      domain.ExecutionReceipt `json:"receipt"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	message, err := h.repository.CompleteTask(r.Context(), identity.RunnerID, domain.TaskID(r.PathValue("taskID")), input.Success, input.Output, input.ErrorSummary, input.ExitCode, input.Receipt)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	h.publishMessage(message)
	writeJSON(w, http.StatusOK, message)
}

// taskCancellation lets a Runner actively observe a user stop request while
// its local CLI is still running. The Runner polls only for tasks it owns and
// has already claimed, so this never exposes another device's task state.
func (h *AutomationHandlers) taskCancellation(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authenticateRunner(w, r)
	if !ok {
		return
	}
	cancelled, err := h.repository.TaskCancellationRequested(r.Context(), identity.RunnerID, domain.TaskID(r.PathValue("taskID")))
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"cancelled": cancelled})
}

func (h *AutomationHandlers) listAgents(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	agents, err := h.repository.ListAgents(r.Context(), domain.WorkspaceID(r.PathValue("workspaceID")), user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

func (h *AutomationHandlers) listAgentCandidates(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	agents, err := h.repository.ListAgentCandidates(r.Context(), domain.WorkspaceID(r.PathValue("workspaceID")), user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

func (h *AutomationHandlers) importAgent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	var input struct {
		AgentID string `json:"agent_id"`
	}
	if err := decodeJSON(w, r, &input); err != nil || strings.TrimSpace(input.AgentID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "agent_id is required")
		return
	}
	if err := h.repository.ImportAgent(r.Context(), domain.WorkspaceID(r.PathValue("workspaceID")), domain.AgentID(input.AgentID), user.ID); err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AutomationHandlers) removeAgent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	if err := h.repository.RemoveAgent(r.Context(), domain.WorkspaceID(r.PathValue("workspaceID")), domain.AgentID(r.PathValue("agentID")), user.ID); err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AutomationHandlers) listRoomAgents(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	agents, err := h.repository.ListRoomAgents(r.Context(), domain.RoomID(r.PathValue("roomID")), user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

func (h *AutomationHandlers) addRoomAgent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	var input struct {
		AgentID string `json:"agent_id"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(input.AgentID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "agent_id is required")
		return
	}
	if err := h.repository.AddRoomAgent(r.Context(), domain.RoomID(r.PathValue("roomID")), domain.AgentID(input.AgentID), user.ID); err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AutomationHandlers) removeRoomAgent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	if err := h.repository.RemoveRoomAgent(r.Context(), domain.RoomID(r.PathValue("roomID")), domain.AgentID(r.PathValue("agentID")), user.ID); err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AutomationHandlers) updateRoomAgentSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	var input struct {
		ResponseMode      domain.AgentResponseMode `json:"response_mode"`
		ReplyDelaySeconds int                      `json:"reply_delay_seconds"`
		RolePrompt        string                   `json:"role_prompt"`
		MaxAutoTurns      int                      `json:"max_auto_turns"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if input.MaxAutoTurns == 0 {
		input.MaxAutoTurns = 4
	}
	validMode := input.ResponseMode == domain.AgentResponseMention || input.ResponseMode == domain.AgentResponseCoordinator || input.ResponseMode == domain.AgentResponseHumanAll || input.ResponseMode == domain.AgentResponseMuted
	if !validMode || input.ReplyDelaySeconds < 0 || input.ReplyDelaySeconds > 60 || input.MaxAutoTurns < 1 ||
		input.MaxAutoTurns > 8 || len(input.RolePrompt) > 2000 {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid response mode, reply delay, role prompt, or max auto turns")
		return
	}
	settings := domain.RoomAgentSettings{RoomID: domain.RoomID(r.PathValue("roomID")), AgentID: domain.AgentID(r.PathValue("agentID")),
		ResponseMode: input.ResponseMode, ReplyDelaySeconds: input.ReplyDelaySeconds, RolePrompt: strings.TrimSpace(input.RolePrompt),
		MaxAutoTurns: input.MaxAutoTurns}
	if err := h.repository.UpdateRoomAgentSettings(r.Context(), settings, user.ID); err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AutomationHandlers) createTask(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	var input struct {
		TargetAgentID      string                     `json:"target_agent_id"`
		Title              string                     `json:"title"`
		Prompt             string                     `json:"prompt"`
		AcceptanceCriteria string                     `json:"acceptance_criteria"`
		PlanStepID         *domain.ProjectStepID      `json:"plan_step_id"`
		ExecutionSource    domain.TaskExecutionSource `json:"execution_source"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Prompt = strings.TrimSpace(input.Prompt)
	if input.TargetAgentID == "" || input.Title == "" || input.Prompt == "" || len(input.Title) > 160 || len(input.Prompt) > 50_000 {
		writeError(w, http.StatusBadRequest, "invalid_request", "target_agent_id, title, and prompt are required")
		return
	}
	if !input.ExecutionSource.Valid() || (input.ExecutionSource == domain.TaskExecutionResponsibleLocal && input.PlanStepID == nil) {
		writeError(w, http.StatusBadRequest, "invalid_request", "选择负责人本机进展时必须同时选择项目步骤")
		return
	}
	id, err := idgen.New("task")
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	now := time.Now().UTC()
	task := domain.Task{ID: domain.TaskID(id), RoomID: domain.RoomID(r.PathValue("roomID")), CreatorKind: domain.TaskCreatorUser, CreatorID: string(user.ID), TargetAgentID: domain.AgentID(input.TargetAgentID), Title: input.Title, Prompt: input.Prompt, AcceptanceCriteria: strings.TrimSpace(input.AcceptanceCriteria), PlanStepID: input.PlanStepID, ExecutionSource: input.ExecutionSource, State: domain.TaskQueued, CurrentAttempt: 1, CreatedAt: now, UpdatedAt: now}
	task, message, err := h.repository.CreateTask(r.Context(), task, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	h.publishMessage(message)
	writeJSON(w, http.StatusCreated, map[string]any{"task": task, "message": message})
}

func (h *AutomationHandlers) listRoomTasks(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	tasks, err := h.repository.ListRoomTasks(r.Context(), domain.RoomID(r.PathValue("roomID")), user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (h *AutomationHandlers) cancelTaskRoot(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	tasks, message, err := h.repository.CancelTasksForRoot(r.Context(), domain.RoomID(r.PathValue("roomID")), domain.MessageID(r.PathValue("rootMessageID")), user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	if message.ID != "" {
		h.publishMessage(message)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks, "message": message})
}

func (h *AutomationHandlers) authenticateUser(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "authentication is required")
		return domain.User{}, false
	}
	user, err := h.auth.Authenticate(r.Context(), cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "authentication is required")
		return domain.User{}, false
	}
	return user, true
}

func (h *AutomationHandlers) authenticateRunner(w http.ResponseWriter, r *http.Request) (domain.RunnerIdentity, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "runner bearer token is required")
		return domain.RunnerIdentity{}, false
	}
	identity, err := h.repository.AuthenticateRunner(r.Context(), auth.HashToken(strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "runner bearer token is invalid")
		return domain.RunnerIdentity{}, false
	}
	return identity, true
}

func (h *AutomationHandlers) publishMessage(message domain.Message) {
	if h.publisher == nil {
		return
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return
	}
	h.publisher.Publish(events.Envelope{EventID: string(message.ID), Type: "message.created", WorkspaceID: string(message.WorkspaceID), RoomID: string(message.RoomID), Sequence: message.Sequence, Sender: events.Sender{Kind: events.SenderKind(message.SenderKind), ID: message.SenderID}, OccurredAt: message.CreatedAt, Payload: payload})
}

func (h *AutomationHandlers) writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "the requested resource was not found")
	case errors.Is(err, auth.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "the requested action is not allowed")
	case errors.Is(err, auth.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "the resource is not in the required state")
	case errors.Is(err, auth.ErrInviteInvalid):
		writeError(w, http.StatusConflict, "invite_invalid", "the Runner invite is invalid, expired, or already used")
	case errors.Is(err, auth.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "the request could not be completed")
	}
}
