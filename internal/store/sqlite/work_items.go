package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/idgen"
)

const workItemSelect = `
SELECT wi.id, wi.workspace_id, wi.room_id, wi.root_message_id, wi.plan_step_id, wi.kind, wi.title, wi.description,
       wi.git_branch, wi.merge_status, wi.merged_commit, wi.branch_commit,
       wi.review_agent_id, wi.verification_agent_id, wi.review_task_id, wi.verification_task_id,
       wi.ai_review_status, wi.ai_review_summary, wi.verification_status, wi.verification_summary,
       wi.status, wi.priority, wi.owner_user_id, wi.due_at, wi.accepted_at, wi.created_at, wi.updated_at,
       (SELECT COUNT(*) FROM tasks t WHERE t.work_item_id = wi.id),
       (SELECT COUNT(*) FROM tasks t WHERE t.work_item_id = wi.id AND t.state IN ('queued', 'dispatched', 'running')),
       (SELECT COUNT(*) FROM tasks t WHERE t.work_item_id = wi.id AND t.state = 'delivered'),
       (SELECT COUNT(*) FROM tasks t WHERE t.work_item_id = wi.id AND t.state IN ('failed', 'blocked')),
       COALESCE((SELECT group_concat(DISTINCT t.target_agent_id) FROM tasks t WHERE t.work_item_id = wi.id), '')
FROM work_items wi`

func (r *CollabRepository) ListRoomWorkItems(ctx context.Context, roomID domain.RoomID, userID domain.UserID) ([]domain.WorkItem, error) {
	items := make([]domain.WorkItem, 0)
	err := r.store.Read(ctx, func(db *sql.DB) error {
		var member int
		if err := db.QueryRowContext(ctx, `SELECT 1 FROM room_members WHERE room_id = ? AND user_id = ?`, roomID, userID).Scan(&member); err != nil {
			return mapNotFound(err)
		}
		rows, err := db.QueryContext(ctx, workItemSelect+`
WHERE wi.room_id = ?
ORDER BY CASE wi.status
    WHEN 'in_progress' THEN 0 WHEN 'review' THEN 1 WHEN 'ready' THEN 2
    WHEN 'backlog' THEN 3 WHEN 'done' THEN 4 ELSE 5 END,
    wi.updated_at DESC, wi.id DESC`, roomID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanWorkItem(rows)
			if err != nil {
				return err
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}

func (r *CollabRepository) UpdateWorkItem(ctx context.Context, itemID domain.WorkItemID, update domain.WorkItemUpdate, userID domain.UserID) (domain.WorkItem, error) {
	var item domain.WorkItem
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		current, err := scanWorkItem(tx.QueryRowContext(ctx, workItemSelect+` WHERE wi.id = ?`, itemID))
		if err != nil {
			return mapNotFound(err)
		}
		if _, err := roomRoleTx(ctx, tx, current.RoomID, userID); err != nil {
			return err
		}

		if update.Title != nil {
			value := strings.TrimSpace(*update.Title)
			if value == "" || len([]rune(value)) > 160 {
				return auth.ErrInvalidInput
			}
			current.Title = value
		}
		if update.Status != nil {
			if !update.Status.Valid() {
				return auth.ErrInvalidInput
			}
			if *update.Status == domain.WorkItemDone && current.ActiveTaskCount > 0 {
				return auth.ErrConflict
			}
			if *update.Status == domain.WorkItemDone && current.MergeStatus == domain.WorkItemMergeOpen {
				return auth.ErrGitMergeRequired
			}
			if *update.Status == domain.WorkItemDone || (current.Status == domain.WorkItemReview && *update.Status == domain.WorkItemInProgress) {
				canManage, err := workspaceProjectAccess(ctx, tx, current.WorkspaceID, userID)
				if err != nil {
					return err
				}
				if !canManage {
					return auth.ErrForbidden
				}
			}
			current.Status = *update.Status
		}
		if update.Priority != nil {
			if !update.Priority.Valid() {
				return auth.ErrInvalidInput
			}
			current.Priority = *update.Priority
		}
		if update.ClearOwner {
			current.OwnerUserID = nil
		} else if update.OwnerUserID != nil {
			var member int
			if err := tx.QueryRowContext(ctx, `SELECT 1 FROM room_members WHERE room_id = ? AND user_id = ?`, current.RoomID, *update.OwnerUserID).Scan(&member); err != nil {
				return mapNotFound(err)
			}
			value := *update.OwnerUserID
			current.OwnerUserID = &value
		}
		if update.ClearDueAt {
			current.DueAt = nil
		} else if update.DueAt != nil {
			value := update.DueAt.UTC()
			current.DueAt = &value
		}
		if update.ClearPlanStep {
			current.PlanStepID = nil
			if current.MergeStatus == domain.WorkItemMergeOpen {
				current.GitBranch = ""
				current.MergeStatus = domain.WorkItemMergeNone
			}
		} else if update.PlanStepID != nil {
			var stepWorkspaceID domain.WorkspaceID
			if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM project_steps WHERE id = ?`, *update.PlanStepID).Scan(&stepWorkspaceID); err != nil {
				return mapNotFound(err)
			}
			if stepWorkspaceID != current.WorkspaceID {
				return auth.ErrInvalidInput
			}
			value := *update.PlanStepID
			current.PlanStepID = &value
		}

		now := time.Now().UTC()
		var owner, dueAt, acceptedAt, planStepID any
		if current.OwnerUserID != nil {
			owner = *current.OwnerUserID
		}
		if current.DueAt != nil {
			dueAt = formatTime(*current.DueAt)
		}
		if current.Status == domain.WorkItemDone {
			if current.AcceptedAt == nil {
				current.AcceptedAt = &now
			}
			acceptedAt = formatTime(*current.AcceptedAt)
		} else {
			current.AcceptedAt = nil
		}
		if current.PlanStepID != nil {
			planStepID = *current.PlanStepID
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE work_items SET title = ?, status = ?, priority = ?, owner_user_id = ?, due_at = ?, accepted_at = ?, plan_step_id = ?, git_branch = ?, merge_status = ?, updated_at = ?
WHERE id = ?`, current.Title, current.Status, current.Priority, owner, dueAt, acceptedAt, planStepID, current.GitBranch, current.MergeStatus, formatTime(now), current.ID); err != nil {
			return err
		}
		item, err = scanWorkItem(tx.QueryRowContext(ctx, workItemSelect+` WHERE wi.id = ?`, itemID))
		return err
	})
	return item, err
}

func (r *CollabRepository) WorkItemForMerge(ctx context.Context, itemID domain.WorkItemID, userID domain.UserID) (domain.WorkItem, error) {
	var item domain.WorkItem
	err := r.store.Read(ctx, func(db *sql.DB) error {
		value, err := scanWorkItem(db.QueryRowContext(ctx, workItemSelect+` WHERE wi.id = ?`, itemID))
		if err != nil {
			return mapNotFound(err)
		}
		canManage, err := workspaceProjectAccess(ctx, db, value.WorkspaceID, userID)
		if err != nil {
			return err
		}
		if !canManage {
			return auth.ErrForbidden
		}
		if value.Status != domain.WorkItemReview || value.MergeStatus != domain.WorkItemMergeOpen || value.GitBranch == "" {
			return auth.ErrConflict
		}
		item = value
		return nil
	})
	return item, err
}

func (r *CollabRepository) WorkItemForSubmission(ctx context.Context, itemID domain.WorkItemID, userID domain.UserID) (domain.WorkItem, error) {
	var item domain.WorkItem
	err := r.store.Read(ctx, func(db *sql.DB) error {
		value, err := scanWorkItem(db.QueryRowContext(ctx, workItemSelect+` WHERE wi.id = ?`, itemID))
		if err != nil {
			return mapNotFound(err)
		}
		var member int
		if err := db.QueryRowContext(ctx, `SELECT 1 FROM room_members WHERE room_id = ? AND user_id = ?`, value.RoomID, userID).Scan(&member); err != nil {
			return mapNotFound(err)
		}
		if value.PlanStepID == nil || value.Status == domain.WorkItemDone || value.Status == domain.WorkItemCancelled {
			return auth.ErrConflict
		}
		if value.GitBranch == "" {
			value.GitBranch = "issue/" + string(value.ID)
			value.MergeStatus = domain.WorkItemMergeOpen
		}
		item = value
		return nil
	})
	return item, err
}

// StartWorkItemAIReview creates a normal Runner task with a special workflow
// role. It is visible in the original topic, but its prompt and immutable
// branch snapshot are server generated so a reviewer cannot accidentally
// inspect main or another task's files.
func (r *CollabRepository) StartWorkItemAIReview(ctx context.Context, itemID domain.WorkItemID, reviewAgentID, verificationAgentID domain.AgentID, commit string, fileCount int, totalBytes int64, userID domain.UserID) (domain.WorkItem, domain.Message, error) {
	var item domain.WorkItem
	var message domain.Message
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		current, err := scanWorkItem(tx.QueryRowContext(ctx, workItemSelect+` WHERE wi.id = ?`, itemID))
		if err != nil {
			return mapNotFound(err)
		}
		canManage, err := workspaceProjectAccess(ctx, tx, current.WorkspaceID, userID)
		if err != nil {
			return err
		}
		if !canManage {
			return auth.ErrForbidden
		}
		if current.Status != domain.WorkItemReview || current.MergeStatus != domain.WorkItemMergeOpen || current.PlanStepID == nil || current.GitBranch == "" || current.BranchCommit == "" {
			return auth.ErrConflict
		}
		for _, agentID := range []domain.AgentID{reviewAgentID, verificationAgentID} {
			var allowed int
			if err := tx.QueryRowContext(ctx, `SELECT 1 FROM room_agents ra JOIN agents a ON a.id = ra.agent_id
				WHERE ra.room_id = ? AND a.id = ? AND a.workspace_id = ? AND a.status != 'unavailable'`, current.RoomID, agentID, current.WorkspaceID).Scan(&allowed); err != nil {
				return mapNotFound(err)
			}
		}
		var sourceName string
		if err := tx.QueryRowContext(ctx, `SELECT source_name FROM project_step_workspaces WHERE step_id = ?`, *current.PlanStepID).Scan(&sourceName); err != nil {
			return mapNotFound(err)
		}
		now := time.Now().UTC()
		task, createdMessage, err := createWorkItemAgentTaskTx(ctx, tx, current, reviewAgentID, userID, "ai_merge_review",
			"AI 合并审查："+current.Title, aiMergeReviewPrompt(current), commit, sourceName, fileCount, totalBytes, now)
		if err != nil {
			return err
		}
		message = createdMessage
		if _, err := tx.ExecContext(ctx, `UPDATE work_items SET review_agent_id = ?, verification_agent_id = ?, review_task_id = ?, verification_task_id = NULL,
			ai_review_status = 'pending', ai_review_summary = '', verification_status = 'none', verification_summary = '', updated_at = ? WHERE id = ?`,
			reviewAgentID, verificationAgentID, task.ID, formatTime(now), itemID); err != nil {
			return err
		}
		item, err = scanWorkItem(tx.QueryRowContext(ctx, workItemSelect+` WHERE wi.id = ?`, itemID))
		return err
	})
	return item, message, err
}

func (r *CollabRepository) RejectWorkItemReview(ctx context.Context, itemID domain.WorkItemID, reason string, userID domain.UserID) (domain.WorkItem, error) {
	var item domain.WorkItem
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		current, err := scanWorkItem(tx.QueryRowContext(ctx, workItemSelect+` WHERE wi.id = ?`, itemID))
		if err != nil {
			return mapNotFound(err)
		}
		canManage, err := workspaceProjectAccess(ctx, tx, current.WorkspaceID, userID)
		if err != nil {
			return err
		}
		if !canManage {
			return auth.ErrForbidden
		}
		if current.Status != domain.WorkItemReview || current.MergeStatus != domain.WorkItemMergeOpen {
			return auth.ErrConflict
		}
		reason = strings.TrimSpace(reason)
		if reason == "" {
			reason = "管理员根据 AI 审查结论退回修改"
		}
		now := time.Now().UTC()
		if _, err := tx.ExecContext(ctx, `UPDATE work_items SET status = 'in_progress', ai_review_status = 'changes_requested',
			ai_review_summary = ?, review_task_id = NULL, verification_task_id = NULL, verification_status = 'none', verification_summary = '', updated_at = ? WHERE id = ?`, reason, formatTime(now), itemID); err != nil {
			return err
		}
		item, err = scanWorkItem(tx.QueryRowContext(ctx, workItemSelect+` WHERE wi.id = ?`, itemID))
		return err
	})
	return item, err
}

func createWorkItemAgentTaskTx(ctx context.Context, tx *sql.Tx, item domain.WorkItem, agentID domain.AgentID, userID domain.UserID, workflowRole, title, prompt, commit, sourceName string, fileCount int, totalBytes int64, now time.Time) (domain.Task, domain.Message, error) {
	var task domain.Task
	var message domain.Message
	var runnerID domain.RunnerID
	var ownerID domain.UserID
	var invocation domain.AgentInvocationScope
	var runnerVersion string
	if err := tx.QueryRowContext(ctx, `SELECT a.runner_id, a.owner_user_id, p.invocation_scope, r.version FROM agents a
		JOIN runners r ON r.id = a.runner_id JOIN room_agents ra ON ra.agent_id = a.id JOIN agent_access_policies p ON p.agent_id = a.id
		WHERE a.id = ? AND a.workspace_id = ? AND ra.room_id = ? AND a.status != 'unavailable' AND p.archived = 0`, agentID, item.WorkspaceID, item.RoomID).Scan(&runnerID, &ownerID, &invocation, &runnerVersion); err != nil {
		return task, message, mapNotFound(err)
	}
	if ownerID != userID && (invocation != domain.AgentInvocationRoomMembers || !runnerSupportsScopedCapabilities(runnerVersion)) {
		return task, message, auth.ErrForbidden
	}
	taskValue, err := idgen.New("task")
	if err != nil {
		return task, message, err
	}
	attemptValue, err := idgen.New("attempt")
	if err != nil {
		return task, message, err
	}
	messageValue, err := idgen.New("msg")
	if err != nil {
		return task, message, err
	}
	// Workflow tasks are children of the same visible topic, but do not reuse
	// the original task source. NULL keeps them distinct even when the selected
	// reviewer was also an executor of the original request.
	var sourceMessageID *domain.MessageID
	task = domain.Task{ID: domain.TaskID(taskValue), WorkItemID: &item.ID, PlanStepID: item.PlanStepID, WorkspaceID: item.WorkspaceID, RoomID: item.RoomID,
		RootMessageID: item.RootMessageID, SourceMessageID: sourceMessageID, CreatorKind: domain.TaskCreatorUser, CreatorID: string(userID), TargetAgentID: agentID,
		Title: title, Prompt: prompt, AcceptanceCriteria: "必须输出真人可读结论和实际验证证据", State: domain.TaskQueued, CurrentAttempt: 1, CreatedAt: now, UpdatedAt: now}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tasks (id, work_item_id, workspace_id, room_id, root_message_id, source_message_id, creator_kind, creator_id,
		target_agent_id, title, prompt, acceptance_criteria, state, current_attempt, workflow_role, project_workspace_commit, project_workspace_source_name,
		project_workspace_revision, project_workspace_file_count, project_workspace_total_bytes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'user', ?, ?, ?, ?, ?, 'queued', 1, ?, ?, ?, 1, ?, ?, ?, ?)`, task.ID, item.ID, item.WorkspaceID, item.RoomID,
		item.RootMessageID, sourceMessageID, userID, agentID, title, prompt, task.AcceptanceCriteria, workflowRole, commit, sourceName, fileCount, totalBytes,
		formatTime(now), formatTime(now)); err != nil {
		return task, message, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO task_attempts (id, task_id, attempt_no, state, created_at) VALUES (?, ?, 1, 'queued', ?)`, attemptValue, task.ID, formatTime(now)); err != nil {
		return task, message, err
	}
	sequence, err := allocateSequence(ctx, tx, item.WorkspaceID)
	if err != nil {
		return task, message, err
	}
	metadata, _ := json.Marshal(map[string]any{"task_id": task.ID, "task_title": title, "task_source": "structured", "workflow_role": workflowRole, "plan_step_id": item.PlanStepID})
	message = domain.Message{ID: domain.MessageID(messageValue), WorkspaceID: item.WorkspaceID, RoomID: item.RoomID, Sequence: sequence, SenderKind: domain.MessageSenderSystem,
		SenderID: "system", Kind: domain.MessageProgress, Body: "已委托 AI 进行技术审查。完成后将在这里返回真人可读结论。", MentionAgentIDs: []domain.AgentID{agentID},
		ReplyToMessageID: item.RootMessageID, RootMessageID: item.RootMessageID, Metadata: metadata, CreatedAt: now}
	if _, err := tx.ExecContext(ctx, `INSERT INTO messages (message_id, workspace_id, room_id, sequence, sender_kind, sender_id, kind, body, mention_agent_ids_json, reply_to_message_id, root_message_id, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, message.ID, message.WorkspaceID, message.RoomID, message.Sequence, message.SenderKind, message.SenderID, message.Kind, message.Body,
		`[`+strconv.Quote(string(agentID))+`]`, message.ReplyToMessageID, message.RootMessageID, string(metadata), formatTime(now)); err != nil {
		return task, message, err
	}
	return task, message, nil
}

func aiMergeReviewPrompt(item domain.WorkItem) string {
	return fmt.Sprintf(`你是本次变更的 AI 技术审查员。请在当前目录中检查待合并分支的完整项目副本，并结合以下业务要求给出真人能看懂的审查结论。

事项：%s
原始要求：%s

必须实际检查文件，并尽可能运行安全、相关的测试或校验。输出严格使用以下标题：
审查结论：建议合并 / 建议修改后再合并 / 不建议合并
完成了什么：用非技术人员能理解的话概括
是否符合要求：说明满足与未满足项
主要风险：没有则写“未发现明显风险”
实际验证：列出真正执行的检查、命令和结果，不得编造
需要管理员决定：列出业务取舍；没有则写“无”

不要执行 git 合并，不要修改共享主项目。`, item.Title, item.Description)
}

func postMergeVerificationPrompt(item domain.WorkItem, commit string) string {
	return fmt.Sprintf(`你是合并后独立校验员。当前目录是已经合并到共享主项目的新版本 %s。请围绕事项“%s”实际检查关键文件并运行适用测试。

输出严格使用以下标题：
校验结论：通过 / 未通过
合并结果：用非技术人员能理解的话说明最终项目是否正常
实际验证：列出真正执行的检查、命令和结果
遗留问题：没有则写“无”

不要修改或再次合并项目。`, commit, item.Title)
}

func (r *CollabRepository) CompleteWorkItemSubmission(ctx context.Context, itemID domain.WorkItemID, commit string, userID domain.UserID) (domain.WorkItem, error) {
	var item domain.WorkItem
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		current, err := scanWorkItem(tx.QueryRowContext(ctx, workItemSelect+` WHERE wi.id = ?`, itemID))
		if err != nil {
			return mapNotFound(err)
		}
		if _, err := roomRoleTx(ctx, tx, current.RoomID, userID); err != nil {
			return err
		}
		now := time.Now().UTC()
		branch := current.GitBranch
		if branch == "" {
			branch = "issue/" + string(current.ID)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE work_items SET git_branch = ?, merge_status = 'open', branch_commit = ?, status = 'review',
			review_agent_id = NULL, verification_agent_id = NULL, review_task_id = NULL, verification_task_id = NULL,
			ai_review_status = 'none', ai_review_summary = '', verification_status = 'none', verification_summary = '', updated_at = ? WHERE id = ?`, branch, strings.TrimSpace(commit), formatTime(now), itemID); err != nil {
			return err
		}
		item, err = scanWorkItem(tx.QueryRowContext(ctx, workItemSelect+` WHERE wi.id = ?`, itemID))
		return err
	})
	return item, err
}

func (r *CollabRepository) CompleteWorkItemMerge(ctx context.Context, itemID domain.WorkItemID, commit string, fileCount int, totalBytes int64, userID domain.UserID) (domain.WorkItem, *domain.Message, error) {
	var item domain.WorkItem
	var verificationMessage *domain.Message
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		current, err := scanWorkItem(tx.QueryRowContext(ctx, workItemSelect+` WHERE wi.id = ?`, itemID))
		if err != nil {
			return mapNotFound(err)
		}
		canManage, err := workspaceProjectAccess(ctx, tx, current.WorkspaceID, userID)
		if err != nil || !canManage {
			if err != nil {
				return err
			}
			return auth.ErrForbidden
		}
		if current.AIReviewStatus != domain.WorkItemAIReviewPassed || current.VerificationAgentID == nil || current.PlanStepID == nil {
			return auth.ErrConflict
		}
		now := time.Now().UTC()
		_, err = tx.ExecContext(ctx, `UPDATE work_items SET merge_status = 'merged', merged_commit = ?, status = 'review', accepted_at = NULL,
			verification_status = 'pending', verification_summary = '', updated_at = ? WHERE id = ? AND merge_status = 'open'`, strings.TrimSpace(commit), formatTime(now), itemID)
		if err != nil {
			return err
		}
		var sourceName string
		if err := tx.QueryRowContext(ctx, `SELECT source_name FROM project_step_workspaces WHERE step_id = ?`, *current.PlanStepID).Scan(&sourceName); err != nil {
			return mapNotFound(err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE project_step_workspaces SET file_count = ?, total_bytes = ?, revision = revision + 1,
			last_commit = ?, updated_by_user_id = ?, updated_at = ? WHERE step_id = ?`, fileCount, totalBytes,
			strings.TrimSpace(commit), userID, formatTime(now), *current.PlanStepID); err != nil {
			return err
		}
		task, message, err := createWorkItemAgentTaskTx(ctx, tx, current, *current.VerificationAgentID, userID, "post_merge_verification",
			"合并后校验："+current.Title, postMergeVerificationPrompt(current, commit), strings.TrimSpace(commit), sourceName,
			fileCount, totalBytes, now)
		if err != nil {
			return err
		}
		verificationMessage = &message
		if _, err := tx.ExecContext(ctx, `UPDATE work_items SET verification_task_id = ? WHERE id = ?`, task.ID, itemID); err != nil {
			return err
		}
		item, err = scanWorkItem(tx.QueryRowContext(ctx, workItemSelect+` WHERE wi.id = ?`, itemID))
		return err
	})
	return item, verificationMessage, err
}

// ensureWorkItemForRootTx creates one durable project item for a visible human
// topic. Every routed or structured Agent task beneath that root reuses it.
func ensureWorkItemForRootTx(ctx context.Context, tx *sql.Tx, workspaceID domain.WorkspaceID, roomID domain.RoomID, rootMessageID domain.MessageID, fallbackTitle string, now time.Time) (domain.WorkItemID, error) {
	var existing domain.WorkItemID
	err := tx.QueryRowContext(ctx, `SELECT id FROM work_items WHERE room_id = ? AND root_message_id = ?`, roomID, rootMessageID).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	var senderKind domain.SenderKind
	var senderID, body, metadataJSON string
	if err := tx.QueryRowContext(ctx, `
SELECT sender_kind, sender_id, body, metadata_json FROM messages
WHERE message_id = ? AND room_id = ?`, rootMessageID, roomID).Scan(&senderKind, &senderID, &body, &metadataJSON); err != nil {
		return "", err
	}
	title := strings.TrimSpace(fallbackTitle)
	var metadata struct {
		TaskTitle  string                `json:"task_title"`
		PlanStepID *domain.ProjectStepID `json:"plan_step_id"`
	}
	_ = json.Unmarshal([]byte(metadataJSON), &metadata)
	if strings.TrimSpace(metadata.TaskTitle) != "" {
		title = strings.TrimSpace(metadata.TaskTitle)
	} else if strings.TrimSpace(body) != "" {
		title = strings.TrimSpace(body)
	}
	if metadata.PlanStepID != nil {
		var stepWorkspaceID domain.WorkspaceID
		if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM project_steps WHERE id = ?`, *metadata.PlanStepID).Scan(&stepWorkspaceID); err != nil {
			return "", mapNotFound(err)
		}
		if stepWorkspaceID != workspaceID {
			return "", auth.ErrInvalidInput
		}
	}
	if runes := []rune(title); len(runes) > 120 {
		title = string(runes[:120]) + "…"
	}
	if title == "" {
		title = "未命名事项"
	}
	kind := domain.WorkItemTask
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(body)), "/brainstorm") {
		kind = domain.WorkItemBrainstorm
	}
	generated, err := idgen.New("work")
	if err != nil {
		return "", err
	}
	var owner any
	if senderKind == domain.MessageSenderUser {
		owner = senderID
	}
	itemID := domain.WorkItemID(generated)
	gitBranch := ""
	mergeStatus := domain.WorkItemMergeNone
	if _, err := tx.ExecContext(ctx, `
INSERT INTO work_items (id, workspace_id, room_id, root_message_id, plan_step_id, git_branch, merge_status, kind, title, description, status, priority, owner_user_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'in_progress', 'normal', ?, ?, ?)`, itemID, workspaceID, roomID, rootMessageID, metadata.PlanStepID, gitBranch, mergeStatus, kind, title, body, owner, formatTime(now), formatTime(now)); err != nil {
		if !isUniqueConstraint(err) {
			return "", err
		}
		if err := tx.QueryRowContext(ctx, `SELECT id FROM work_items WHERE room_id = ? AND root_message_id = ?`, roomID, rootMessageID).Scan(&itemID); err != nil {
			return "", err
		}
	}
	return itemID, nil
}

func syncWorkItemStateTx(ctx context.Context, tx *sql.Tx, itemID domain.WorkItemID, now time.Time) error {
	if itemID == "" {
		return nil
	}
	var total, active, delivered, cancelled int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*),
       SUM(CASE WHEN state IN ('queued', 'dispatched', 'running') THEN 1 ELSE 0 END),
       SUM(CASE WHEN state = 'delivered' THEN 1 ELSE 0 END),
       SUM(CASE WHEN state = 'cancelled' THEN 1 ELSE 0 END)
FROM tasks WHERE work_item_id = ?`, itemID).Scan(&total, &active, &delivered, &cancelled); err != nil {
		return err
	}
	status := domain.WorkItemInProgress
	if total > 0 && cancelled == total {
		status = domain.WorkItemCancelled
	} else if active == 0 && delivered > 0 {
		status = domain.WorkItemReview
	}
	_, err := tx.ExecContext(ctx, `
UPDATE work_items SET status = ?, updated_at = ?
WHERE id = ? AND status NOT IN ('done') AND verification_status NOT IN ('pending', 'passed', 'failed')`, status, formatTime(now), itemID)
	return err
}

func scanWorkItem(row rowScanner) (domain.WorkItem, error) {
	var item domain.WorkItem
	var rootMessageID, planStepID, ownerUserID, dueAt, acceptedAt sql.NullString
	var reviewAgentID, verificationAgentID, reviewTaskID, verificationTaskID sql.NullString
	var createdAt, updatedAt, agentIDs string
	if err := row.Scan(&item.ID, &item.WorkspaceID, &item.RoomID, &rootMessageID, &planStepID, &item.Kind, &item.Title, &item.Description,
		&item.GitBranch, &item.MergeStatus, &item.MergedCommit, &item.BranchCommit,
		&reviewAgentID, &verificationAgentID, &reviewTaskID, &verificationTaskID, &item.AIReviewStatus, &item.AIReviewSummary, &item.VerificationStatus, &item.VerificationSummary,
		&item.Status, &item.Priority, &ownerUserID, &dueAt, &acceptedAt, &createdAt, &updatedAt,
		&item.TaskCount, &item.ActiveTaskCount, &item.DeliveredTaskCount, &item.FailedTaskCount, &agentIDs); err != nil {
		return item, err
	}
	if rootMessageID.Valid {
		value := domain.MessageID(rootMessageID.String)
		item.RootMessageID = &value
	}
	if planStepID.Valid {
		value := domain.ProjectStepID(planStepID.String)
		item.PlanStepID = &value
	}
	if reviewAgentID.Valid {
		value := domain.AgentID(reviewAgentID.String)
		item.ReviewAgentID = &value
	}
	if verificationAgentID.Valid {
		value := domain.AgentID(verificationAgentID.String)
		item.VerificationAgentID = &value
	}
	if reviewTaskID.Valid {
		value := domain.TaskID(reviewTaskID.String)
		item.ReviewTaskID = &value
	}
	if verificationTaskID.Valid {
		value := domain.TaskID(verificationTaskID.String)
		item.VerificationTaskID = &value
	}
	if ownerUserID.Valid {
		value := domain.UserID(ownerUserID.String)
		item.OwnerUserID = &value
	}
	if dueAt.Valid {
		value, err := time.Parse(time.RFC3339Nano, dueAt.String)
		if err != nil {
			return item, fmt.Errorf("parse work item due_at: %w", err)
		}
		item.DueAt = &value
	}
	if acceptedAt.Valid {
		value, err := time.Parse(time.RFC3339Nano, acceptedAt.String)
		if err != nil {
			return item, fmt.Errorf("parse work item accepted_at: %w", err)
		}
		item.AcceptedAt = &value
	}
	var err error
	if item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return item, fmt.Errorf("parse work item created_at: %w", err)
	}
	if item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return item, fmt.Errorf("parse work item updated_at: %w", err)
	}
	item.TargetAgentIDs = []domain.AgentID{}
	for _, id := range strings.Split(agentIDs, ",") {
		if id = strings.TrimSpace(id); id != "" {
			item.TargetAgentIDs = append(item.TargetAgentIDs, domain.AgentID(id))
		}
	}
	return item, nil
}
