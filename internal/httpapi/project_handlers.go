package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/storage"
)

type ProjectRepository interface {
	ListRoomWorkItems(context.Context, domain.RoomID, domain.UserID) ([]domain.WorkItem, error)
	UpdateWorkItem(context.Context, domain.WorkItemID, domain.WorkItemUpdate, domain.UserID) (domain.WorkItem, error)
	GetWorkspaceProjectPlan(context.Context, domain.WorkspaceID, domain.UserID) (domain.ProjectPlan, error)
	UpdateWorkspaceProjectPlan(context.Context, domain.WorkspaceID, domain.ProjectPlanUpdate, domain.UserID) (domain.ProjectPlan, error)
	CreateProjectStep(context.Context, domain.WorkspaceID, domain.ProjectStepCreate, domain.UserID) (domain.ProjectStep, error)
	UpdateProjectStep(context.Context, domain.ProjectStepID, domain.ProjectStepUpdate, domain.UserID) (domain.ProjectStep, error)
	DeleteProjectStep(context.Context, domain.ProjectStepID, domain.UserID) error
	UpdateRoomProjectStep(context.Context, domain.RoomID, domain.RoomProjectStepUpdate, domain.UserID) error
	WorkItemForMerge(context.Context, domain.WorkItemID, domain.UserID) (domain.WorkItem, error)
	CompleteWorkItemMerge(context.Context, domain.WorkItemID, string, int, int64, domain.UserID) (domain.WorkItem, *domain.Message, error)
	StartWorkItemAIReview(context.Context, domain.WorkItemID, domain.AgentID, domain.AgentID, string, int, int64, domain.UserID) (domain.WorkItem, domain.Message, error)
	RejectWorkItemReview(context.Context, domain.WorkItemID, string, domain.UserID) (domain.WorkItem, error)
	WorkItemForSubmission(context.Context, domain.WorkItemID, domain.UserID) (domain.WorkItem, error)
	CompleteWorkItemSubmission(context.Context, domain.WorkItemID, string, domain.UserID) (domain.WorkItem, error)
	ProjectStepForWorkspaceRead(context.Context, domain.ProjectStepID, domain.UserID) (domain.ProjectStep, error)
	ProjectStepForWorkspaceUpdate(context.Context, domain.ProjectStepID, domain.UserID) (domain.ProjectStep, error)
	SaveProjectStepWorkspace(context.Context, domain.ProjectStepID, string, int, int64, string, domain.UserID) (domain.ProjectStep, error)
	RemoveProjectStepWorkspace(context.Context, domain.ProjectStepID, domain.UserID) (domain.ProjectStep, error)
	SaveProjectStepLocalBinding(context.Context, domain.ProjectStepID, domain.ProjectStepLocalBindingUpdate, domain.UserID) (domain.ProjectStep, error)
	RemoveProjectStepLocalBinding(context.Context, domain.ProjectStepID, domain.RunnerID, domain.UserID) (domain.ProjectStep, error)
}

type ProjectHandlers struct {
	auth        *auth.Service
	repository  ProjectRepository
	storageRoot string
}

func NewProjectHandlers(authService *auth.Service, repository ProjectRepository, storageRoots ...string) *ProjectHandlers {
	handler := &ProjectHandlers{auth: authService, repository: repository}
	if len(storageRoots) > 0 {
		handler.storageRoot = storageRoots[0]
	}
	return handler
}

func (h *ProjectHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/rooms/{roomID}/work-items", h.listRoomWorkItems)
	mux.HandleFunc("PATCH /api/v1/work-items/{workItemID}", h.updateWorkItem)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/project-plan", h.getProjectPlan)
	mux.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}/project-plan", h.updateProjectPlan)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/project-steps", h.createProjectStep)
	mux.HandleFunc("PATCH /api/v1/project-steps/{stepID}", h.updateProjectStep)
	mux.HandleFunc("DELETE /api/v1/project-steps/{stepID}", h.deleteProjectStep)
	mux.HandleFunc("PATCH /api/v1/rooms/{roomID}/project-step", h.updateRoomProjectStep)
	mux.HandleFunc("GET /api/v1/work-items/{workItemID}/review", h.reviewWorkItem)
	mux.HandleFunc("POST /api/v1/work-items/{workItemID}/merge", h.mergeWorkItem)
	mux.HandleFunc("POST /api/v1/work-items/{workItemID}/ai-review", h.startAIReview)
	mux.HandleFunc("POST /api/v1/work-items/{workItemID}/reject", h.rejectWorkItem)
	mux.HandleFunc("POST /api/v1/work-items/{workItemID}/submit-workspace", h.submitWorkItemWorkspace)
	mux.HandleFunc("GET /api/v1/project-steps/{stepID}/workspace", h.downloadProjectStepWorkspace)
	mux.HandleFunc("POST /api/v1/project-steps/{stepID}/workspace", h.uploadProjectStepWorkspace)
	mux.HandleFunc("DELETE /api/v1/project-steps/{stepID}/workspace", h.removeProjectStepWorkspace)
	mux.HandleFunc("PUT /api/v1/project-steps/{stepID}/local-bindings/current", h.saveProjectStepLocalBinding)
	mux.HandleFunc("DELETE /api/v1/project-steps/{stepID}/local-bindings/current/{runnerID}", h.removeProjectStepLocalBinding)
}

func (h *ProjectHandlers) saveProjectStepLocalBinding(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	var input domain.ProjectStepLocalBindingUpdate
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	updated, err := h.repository.SaveProjectStepLocalBinding(r.Context(), domain.ProjectStepID(r.PathValue("stepID")), input, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *ProjectHandlers) removeProjectStepLocalBinding(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	updated, err := h.repository.RemoveProjectStepLocalBinding(r.Context(), domain.ProjectStepID(r.PathValue("stepID")), domain.RunnerID(r.PathValue("runnerID")), user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *ProjectHandlers) startAIReview(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	var input struct {
		ReviewAgentID       domain.AgentID `json:"review_agent_id"`
		VerificationAgentID domain.AgentID `json:"verification_agent_id"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if input.ReviewAgentID == "" || input.VerificationAgentID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "请选择审查 Agent 和合并后校验 Agent")
		return
	}
	item, err := h.repository.WorkItemForMerge(r.Context(), domain.WorkItemID(r.PathValue("workItemID")), user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	if item.PlanStepID == nil {
		writeError(w, http.StatusConflict, "step_required", "该任务尚未关联项目步骤")
		return
	}
	review, err := storage.ReviewWorkspaceGitBranch(h.storageRoot, string(item.WorkspaceID), string(*item.PlanStepID), item.GitBranch)
	if err != nil {
		h.writeWorkspaceError(w, err)
		return
	}
	if !review.Mergeable {
		writeError(w, http.StatusConflict, "merge_conflict", "分支与 main 存在冲突，不能发起 AI 审查")
		return
	}
	snapshot, err := storage.InspectProjectStepSnapshot(h.storageRoot, string(item.WorkspaceID), string(*item.PlanStepID), review.HeadCommit)
	if err != nil {
		h.writeWorkspaceError(w, err)
		return
	}
	updated, message, err := h.repository.StartWorkItemAIReview(r.Context(), item.ID, input.ReviewAgentID, input.VerificationAgentID, snapshot.Commit, snapshot.FileCount, snapshot.TotalBytes, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	_ = message // polling/realtime reconciliation will expose the persisted progress message
	writeJSON(w, http.StatusCreated, updated)
}

func (h *ProjectHandlers) rejectWorkItem(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	updated, err := h.repository.RejectWorkItemReview(r.Context(), domain.WorkItemID(r.PathValue("workItemID")), input.Reason, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *ProjectHandlers) reviewWorkItem(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	if h.storageRoot == "" {
		writeError(w, http.StatusServiceUnavailable, "repository_unavailable", "Server 尚未配置 Git 项目存储")
		return
	}
	item, err := h.repository.WorkItemForMerge(r.Context(), domain.WorkItemID(r.PathValue("workItemID")), user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	if item.PlanStepID == nil {
		writeError(w, http.StatusConflict, "step_required", "该任务尚未关联项目步骤")
		return
	}
	review, err := storage.ReviewWorkspaceGitBranch(h.storageRoot, string(item.WorkspaceID), string(*item.PlanStepID), item.GitBranch)
	if err != nil {
		h.writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, review)
}

func (h *ProjectHandlers) downloadProjectStepWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	if h.storageRoot == "" {
		writeError(w, http.StatusServiceUnavailable, "repository_unavailable", "Server 尚未配置项目存储目录")
		return
	}
	step, err := h.repository.ProjectStepForWorkspaceRead(r.Context(), domain.ProjectStepID(r.PathValue("stepID")), user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	if step.Workspace == nil || strings.TrimSpace(step.Workspace.LastCommit) == "" {
		writeError(w, http.StatusNotFound, "workspace_not_found", "这个步骤还没有可下载的共享底稿")
		return
	}
	archive, err := storage.CreateProjectStepArchive(h.storageRoot, string(step.WorkspaceID), string(step.ID), step.Workspace.LastCommit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "project_workspace_unavailable", "无法生成步骤底稿下载包")
		return
	}
	defer archive.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="agentcollab-step-workspace.zip"`)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", archive.Size))
	w.Header().Set("X-AgentCollab-Project-Commit", step.Workspace.LastCommit)
	w.Header().Set("X-AgentCollab-Project-Step", string(step.ID))
	http.ServeContent(w, r, "agentcollab-step-workspace.zip", time.Time{}, archive)
}

func (h *ProjectHandlers) uploadProjectStepWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	if h.storageRoot == "" {
		writeError(w, http.StatusServiceUnavailable, "repository_unavailable", "Server 尚未配置项目存储目录")
		return
	}
	step, err := h.repository.ProjectStepForWorkspaceUpdate(r.Context(), domain.ProjectStepID(r.PathValue("stepID")), user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	uploads, sourceName, cleanup, err := parseWorkspaceUpload(r)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_workspace", err.Error())
		return
	}
	snapshot, err := storage.CommitStepWorkspace(h.storageRoot, string(step.WorkspaceID), string(step.ID), "main", user.DisplayName, "Update step workspace: "+step.Title, uploads)
	if err != nil {
		h.writeWorkspaceError(w, err)
		return
	}
	updated, err := h.repository.SaveProjectStepWorkspace(r.Context(), step.ID, sourceName, snapshot.FileCount, snapshot.TotalBytes, snapshot.Commit, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *ProjectHandlers) removeProjectStepWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	step, err := h.repository.ProjectStepForWorkspaceUpdate(r.Context(), domain.ProjectStepID(r.PathValue("stepID")), user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	if step.Workspace == nil {
		writeJSON(w, http.StatusOK, step)
		return
	}
	// This removes the active binding only. The Server Git history remains the
	// recovery and audit trail, so an accidental unlink is reversible by a new
	// directory upload and never rewrites historical task evidence.
	updated, err := h.repository.RemoveProjectStepWorkspace(r.Context(), step.ID, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *ProjectHandlers) submitWorkItemWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	if h.storageRoot == "" {
		writeError(w, http.StatusServiceUnavailable, "repository_unavailable", "Server 尚未配置项目存储目录")
		return
	}
	item, err := h.repository.WorkItemForSubmission(r.Context(), domain.WorkItemID(r.PathValue("workItemID")), user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	uploads, _, cleanup, err := parseWorkspaceUpload(r)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_workspace", err.Error())
		return
	}
	snapshot, err := storage.CommitStepWorkspace(h.storageRoot, string(item.WorkspaceID), string(*item.PlanStepID), item.GitBranch, user.DisplayName, "Submit for review: "+item.Title, uploads)
	if err != nil {
		h.writeWorkspaceError(w, err)
		return
	}
	updated, err := h.repository.CompleteWorkItemSubmission(r.Context(), item.ID, snapshot.Commit, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func parseWorkspaceUpload(r *http.Request) ([]storage.WorkspaceUpload, string, func(), error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, "", nil, fmt.Errorf("上传格式无效，请重新选择目录")
	}
	tempDir, err := os.MkdirTemp("", "agentcollab-upload-")
	if err != nil {
		return nil, "", nil, fmt.Errorf("Server 无法准备上传缓存")
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	type stagedFile struct {
		path string
		size int64
	}
	staged := make([]stagedFile, 0)
	paths := make([]string, 0)
	sourceName := ""
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			cleanup()
			return nil, "", nil, fmt.Errorf("目录上传中断，请检查连接后重试")
		}
		switch part.FormName() {
		case "source_name":
			value, readErr := io.ReadAll(io.LimitReader(part, 4097))
			if readErr != nil || len(value) > 4096 {
				part.Close()
				cleanup()
				return nil, "", nil, fmt.Errorf("目录名称无效")
			}
			sourceName = strings.TrimSpace(string(value))
		case "paths":
			value, readErr := io.ReadAll(io.LimitReader(part, 32769))
			if readErr != nil || len(value) > 32768 {
				part.Close()
				cleanup()
				return nil, "", nil, fmt.Errorf("目录中包含无效文件路径")
			}
			paths = append(paths, string(value))
		case "files":
			file, createErr := os.CreateTemp(tempDir, "file-")
			if createErr != nil {
				part.Close()
				cleanup()
				return nil, "", nil, fmt.Errorf("Server 无法保存上传缓存")
			}
			size, copyErr := io.Copy(file, part)
			closeErr := file.Close()
			if copyErr != nil || closeErr != nil {
				part.Close()
				cleanup()
				return nil, "", nil, fmt.Errorf("目录上传中断，请检查磁盘空间和连接")
			}
			staged = append(staged, stagedFile{path: file.Name(), size: size})
		}
		part.Close()
	}
	if len(staged) == 0 || len(staged) != len(paths) {
		cleanup()
		return nil, "", nil, fmt.Errorf("请选择包含文件的有效目录")
	}
	uploads := make([]storage.WorkspaceUpload, 0, len(staged))
	for index, file := range staged {
		stagedPath := file.path
		uploads = append(uploads, storage.WorkspaceUpload{RelativePath: paths[index], Size: file.size, Open: func() (io.ReadCloser, error) { return os.Open(stagedPath) }})
	}
	return uploads, sourceName, cleanup, nil
}

func (h *ProjectHandlers) writeWorkspaceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrEmptyWorkspace):
		writeError(w, http.StatusBadRequest, "empty_workspace", "所选目录没有可提交的文件")
	case errors.Is(err, storage.ErrGitMergeConflict):
		writeError(w, http.StatusConflict, "workspace_conflict", "任务分支与主工作区存在冲突")
	default:
		writeError(w, http.StatusInternalServerError, "workspace_failed", "无法保存目录，请检查文件权限后重试")
	}
}

func (h *ProjectHandlers) mergeWorkItem(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	if h.storageRoot == "" {
		writeError(w, http.StatusServiceUnavailable, "repository_unavailable", "Server 尚未配置 Git 项目存储")
		return
	}
	itemID := domain.WorkItemID(r.PathValue("workItemID"))
	item, err := h.repository.WorkItemForMerge(r.Context(), itemID, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	if item.AIReviewStatus != domain.WorkItemAIReviewPassed || item.VerificationAgentID == nil {
		writeError(w, http.StatusConflict, "ai_review_required", "请先完成 AI 审查并选择合并后校验 Agent")
		return
	}
	commit, err := storage.MergeWorkspaceGitBranch(h.storageRoot, string(item.WorkspaceID), item.GitBranch, user.DisplayName)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrGitBranchNotFound):
			writeError(w, http.StatusConflict, "branch_not_pushed", "功能分支尚未推送到工作区主仓")
		case errors.Is(err, storage.ErrGitMergeConflict):
			writeError(w, http.StatusConflict, "merge_conflict", "分支与 main 存在冲突，请更新分支后重新提交审查")
		default:
			writeError(w, http.StatusInternalServerError, "merge_failed", "Server 无法合并该分支")
		}
		return
	}
	snapshot, err := storage.InspectProjectStepSnapshot(h.storageRoot, string(item.WorkspaceID), string(*item.PlanStepID), commit)
	if err != nil {
		h.writeWorkspaceError(w, err)
		return
	}
	merged, verificationMessage, err := h.repository.CompleteWorkItemMerge(r.Context(), itemID, commit, snapshot.FileCount, snapshot.TotalBytes, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	_ = verificationMessage // persisted progress message is visible on the next room sync
	writeJSON(w, http.StatusOK, merged)
}

func (h *ProjectHandlers) updateRoomProjectStep(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	var input domain.RoomProjectStepUpdate
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := h.repository.UpdateRoomProjectStep(r.Context(), domain.RoomID(r.PathValue("roomID")), input, user.ID); err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ProjectHandlers) getProjectPlan(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	plan, err := h.repository.GetWorkspaceProjectPlan(r.Context(), domain.WorkspaceID(r.PathValue("workspaceID")), user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (h *ProjectHandlers) updateProjectPlan(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	var input domain.ProjectPlanUpdate
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	plan, err := h.repository.UpdateWorkspaceProjectPlan(r.Context(), domain.WorkspaceID(r.PathValue("workspaceID")), input, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (h *ProjectHandlers) createProjectStep(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	var input domain.ProjectStepCreate
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	step, err := h.repository.CreateProjectStep(r.Context(), domain.WorkspaceID(r.PathValue("workspaceID")), input, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, step)
}

func (h *ProjectHandlers) updateProjectStep(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	var input domain.ProjectStepUpdate
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	step, err := h.repository.UpdateProjectStep(r.Context(), domain.ProjectStepID(r.PathValue("stepID")), input, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, step)
}

func (h *ProjectHandlers) deleteProjectStep(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	if err := h.repository.DeleteProjectStep(r.Context(), domain.ProjectStepID(r.PathValue("stepID")), user.ID); err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ProjectHandlers) listRoomWorkItems(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	items, err := h.repository.ListRoomWorkItems(r.Context(), domain.RoomID(r.PathValue("roomID")), user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"work_items": items})
}

func (h *ProjectHandlers) updateWorkItem(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticateUser(w, r)
	if !ok {
		return
	}
	var input struct {
		Title         *string                  `json:"title"`
		Status        *domain.WorkItemStatus   `json:"status"`
		Priority      *domain.WorkItemPriority `json:"priority"`
		OwnerUserID   *domain.UserID           `json:"owner_user_id"`
		ClearOwner    bool                     `json:"clear_owner"`
		DueAt         *string                  `json:"due_at"`
		ClearDueAt    bool                     `json:"clear_due_at"`
		PlanStepID    *domain.ProjectStepID    `json:"plan_step_id"`
		ClearPlanStep bool                     `json:"clear_plan_step"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	update := domain.WorkItemUpdate{Title: input.Title, Status: input.Status, Priority: input.Priority, OwnerUserID: input.OwnerUserID, ClearOwner: input.ClearOwner, ClearDueAt: input.ClearDueAt, PlanStepID: input.PlanStepID, ClearPlanStep: input.ClearPlanStep}
	if input.DueAt != nil && strings.TrimSpace(*input.DueAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*input.DueAt))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "due_at must be an RFC3339 timestamp")
			return
		}
		update.DueAt = &parsed
	}
	item, err := h.repository.UpdateWorkItem(r.Context(), domain.WorkItemID(r.PathValue("workItemID")), update, user.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *ProjectHandlers) authenticateUser(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
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

func (h *ProjectHandlers) writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "the requested resource was not found")
	case errors.Is(err, auth.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "the requested action is not allowed")
	case errors.Is(err, auth.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "仍有 Agent 正在执行，结束或停止后才能验收完成")
	case errors.Is(err, auth.ErrGitMergeRequired):
		writeError(w, http.StatusConflict, "git_merge_required", "该事项已有功能分支，请先进入待验收，再使用“审查并合并”完成")
	case errors.Is(err, auth.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "工作事项设置无效")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "the request could not be completed")
	}
}
