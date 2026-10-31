package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/idgen"
)

type projectQueryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func workspaceProjectAccess(ctx context.Context, queryer projectQueryRower, workspaceID domain.WorkspaceID, userID domain.UserID) (bool, error) {
	var role domain.WorkspaceRole
	if err := queryer.QueryRowContext(ctx, `SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, userID).Scan(&role); err != nil {
		return false, mapNotFound(err)
	}
	return role == domain.WorkspaceOwner || role == domain.WorkspaceAdmin, nil
}

func (r *CollabRepository) GetWorkspaceProjectPlan(ctx context.Context, workspaceID domain.WorkspaceID, userID domain.UserID) (domain.ProjectPlan, error) {
	var plan domain.ProjectPlan
	err := r.store.Read(ctx, func(db *sql.DB) error {
		canManage, err := workspaceProjectAccess(ctx, db, workspaceID, userID)
		if err != nil {
			return err
		}
		plan = domain.ProjectPlan{WorkspaceID: workspaceID, Status: domain.ProjectPlanPlanning, CanManage: canManage, Steps: []domain.ProjectStep{}}
		var createdAt, updatedAt string
		err = db.QueryRowContext(ctx, `SELECT objective, status, created_at, updated_at FROM project_plans WHERE workspace_id = ?`, workspaceID).Scan(&plan.Objective, &plan.Status, &createdAt, &updatedAt)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			created, parseErr := time.Parse(time.RFC3339Nano, createdAt)
			if parseErr != nil {
				return fmt.Errorf("parse project plan created_at: %w", parseErr)
			}
			updated, parseErr := time.Parse(time.RFC3339Nano, updatedAt)
			if parseErr != nil {
				return fmt.Errorf("parse project plan updated_at: %w", parseErr)
			}
			plan.CreatedAt, plan.UpdatedAt = &created, &updated
		}
		rows, err := db.QueryContext(ctx, `
SELECT ps.id, ps.workspace_id, ps.parent_step_id, ps.title, ps.description, ps.weight, ps.status,
       ps.sort_order, ps.created_at, ps.updated_at, COUNT(wi.id)
FROM project_steps ps LEFT JOIN work_items wi ON wi.plan_step_id = ps.id
WHERE ps.workspace_id = ? GROUP BY ps.id ORDER BY ps.sort_order, ps.created_at, ps.id`, workspaceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			step, scanErr := scanProjectStep(rows)
			if scanErr != nil {
				return scanErr
			}
			plan.Steps = append(plan.Steps, step)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := loadProjectStepResponsibles(ctx, db, workspaceID, plan.Steps); err != nil {
			return err
		}
		if err := loadProjectStepWorkspaces(ctx, db, workspaceID, plan.Steps); err != nil {
			return err
		}
		if err := loadProjectStepLocalBindings(ctx, db, workspaceID, plan.Steps); err != nil {
			return err
		}
		plan.Progress = calculateProjectProgress(plan.Steps)
		return nil
	})
	return plan, err
}

func (r *CollabRepository) UpdateWorkspaceProjectPlan(ctx context.Context, workspaceID domain.WorkspaceID, update domain.ProjectPlanUpdate, userID domain.UserID) (domain.ProjectPlan, error) {
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		canManage, err := workspaceProjectAccess(ctx, tx, workspaceID, userID)
		if err != nil {
			return err
		}
		if !canManage {
			return auth.ErrForbidden
		}
		objective, status := "", domain.ProjectPlanPlanning
		var existingObjective string
		var existingStatus domain.ProjectPlanStatus
		err = tx.QueryRowContext(ctx, `SELECT objective, status FROM project_plans WHERE workspace_id = ?`, workspaceID).Scan(&existingObjective, &existingStatus)
		if err == nil {
			objective, status = existingObjective, existingStatus
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if update.Objective != nil {
			objective = strings.TrimSpace(*update.Objective)
			if len([]rune(objective)) > 5000 {
				return auth.ErrInvalidInput
			}
		}
		if update.Status != nil {
			if !update.Status.Valid() {
				return auth.ErrInvalidInput
			}
			status = *update.Status
		}
		now := time.Now().UTC()
		_, err = tx.ExecContext(ctx, `INSERT INTO project_plans (workspace_id, objective, status, created_by_user_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(workspace_id) DO UPDATE SET objective = excluded.objective, status = excluded.status, updated_at = excluded.updated_at`,
			workspaceID, objective, status, userID, formatTime(now), formatTime(now))
		return err
	})
	if err != nil {
		return domain.ProjectPlan{}, err
	}
	return r.GetWorkspaceProjectPlan(ctx, workspaceID, userID)
}

func (r *CollabRepository) CreateProjectStep(ctx context.Context, workspaceID domain.WorkspaceID, input domain.ProjectStepCreate, userID domain.UserID) (domain.ProjectStep, error) {
	var stepID domain.ProjectStepID
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		canManage, err := workspaceProjectAccess(ctx, tx, workspaceID, userID)
		if err != nil {
			return err
		}
		if !canManage {
			return auth.ErrForbidden
		}
		title, description := strings.TrimSpace(input.Title), strings.TrimSpace(input.Description)
		if title == "" || len([]rune(title)) > 160 || len([]rune(description)) > 5000 || input.Weight < 1 || input.Weight > 1000 {
			return auth.ErrInvalidInput
		}
		status := input.Status
		if status == "" {
			status = domain.ProjectStepPlanned
		}
		if !status.Valid() {
			return auth.ErrInvalidInput
		}
		if input.ParentStepID != nil {
			var parentWorkspaceID domain.WorkspaceID
			if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM project_steps WHERE id = ?`, *input.ParentStepID).Scan(&parentWorkspaceID); err != nil {
				return mapNotFound(err)
			}
			if parentWorkspaceID != workspaceID {
				return auth.ErrInvalidInput
			}
		}
		generated, err := idgen.New("step")
		if err != nil {
			return err
		}
		stepID = domain.ProjectStepID(generated)
		now := time.Now().UTC()
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_plans (workspace_id, objective, status, created_by_user_id, created_at, updated_at)
VALUES (?, '', 'planning', ?, ?, ?) ON CONFLICT(workspace_id) DO NOTHING`, workspaceID, userID, formatTime(now), formatTime(now)); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO project_steps (id, workspace_id, parent_step_id, title, description, weight, status, sort_order, created_by_user_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, stepID, workspaceID, input.ParentStepID, title, description, input.Weight, status, input.SortOrder, userID, formatTime(now), formatTime(now))
		if err != nil {
			return err
		}
		return replaceProjectStepResponsibles(ctx, tx, stepID, workspaceID, input.ResponsibleUserIDs, input.ResponsibleAgentIDs, userID)
	})
	if err != nil {
		return domain.ProjectStep{}, err
	}
	return r.getProjectStep(ctx, stepID, userID)
}

func (r *CollabRepository) UpdateProjectStep(ctx context.Context, stepID domain.ProjectStepID, update domain.ProjectStepUpdate, userID domain.UserID) (domain.ProjectStep, error) {
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		var current domain.ProjectStep
		var parentID sql.NullString
		var createdAt, updatedAt string
		if err := tx.QueryRowContext(ctx, `SELECT id, workspace_id, parent_step_id, title, description, weight, status, sort_order, created_at, updated_at, 0 FROM project_steps WHERE id = ?`, stepID).Scan(
			&current.ID, &current.WorkspaceID, &parentID, &current.Title, &current.Description, &current.Weight, &current.Status, &current.SortOrder, &createdAt, &updatedAt, &current.LinkedItemCount); err != nil {
			return mapNotFound(err)
		}
		canManage, err := workspaceProjectAccess(ctx, tx, current.WorkspaceID, userID)
		if err != nil {
			return err
		}
		if !canManage {
			return auth.ErrForbidden
		}
		if parentID.Valid {
			value := domain.ProjectStepID(parentID.String)
			current.ParentStepID = &value
		}
		if update.Title != nil {
			value := strings.TrimSpace(*update.Title)
			if value == "" || len([]rune(value)) > 160 {
				return auth.ErrInvalidInput
			}
			current.Title = value
		}
		if update.Description != nil {
			value := strings.TrimSpace(*update.Description)
			if len([]rune(value)) > 5000 {
				return auth.ErrInvalidInput
			}
			current.Description = value
		}
		if update.Weight != nil {
			if *update.Weight < 1 || *update.Weight > 1000 {
				return auth.ErrInvalidInput
			}
			current.Weight = *update.Weight
		}
		if update.Status != nil {
			if !update.Status.Valid() {
				return auth.ErrInvalidInput
			}
			current.Status = *update.Status
		}
		if update.SortOrder != nil {
			current.SortOrder = *update.SortOrder
		}
		if update.ClearParent {
			current.ParentStepID = nil
		} else if update.ParentStepID != nil {
			if *update.ParentStepID == stepID {
				return auth.ErrInvalidInput
			}
			var parentWorkspaceID domain.WorkspaceID
			if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM project_steps WHERE id = ?`, *update.ParentStepID).Scan(&parentWorkspaceID); err != nil {
				return mapNotFound(err)
			}
			if parentWorkspaceID != current.WorkspaceID {
				return auth.ErrInvalidInput
			}
			var cycle int
			if err := tx.QueryRowContext(ctx, `WITH RECURSIVE descendants(id) AS (SELECT id FROM project_steps WHERE parent_step_id = ? UNION ALL SELECT ps.id FROM project_steps ps JOIN descendants d ON ps.parent_step_id = d.id) SELECT COUNT(*) FROM descendants WHERE id = ?`, stepID, *update.ParentStepID).Scan(&cycle); err != nil {
				return err
			}
			if cycle > 0 {
				return auth.ErrInvalidInput
			}
			value := *update.ParentStepID
			current.ParentStepID = &value
		}
		var parent any
		if current.ParentStepID != nil {
			parent = *current.ParentStepID
		}
		_, err = tx.ExecContext(ctx, `UPDATE project_steps SET parent_step_id = ?, title = ?, description = ?, weight = ?, status = ?, sort_order = ?, updated_at = ? WHERE id = ?`, parent, current.Title, current.Description, current.Weight, current.Status, current.SortOrder, formatTime(time.Now().UTC()), stepID)
		if err != nil {
			return err
		}
		if update.ResponsibleUserIDs != nil {
			if err := replaceProjectStepUserResponsibles(ctx, tx, stepID, current.WorkspaceID, *update.ResponsibleUserIDs, userID); err != nil {
				return err
			}
		}
		if update.ResponsibleAgentIDs != nil {
			if err := replaceProjectStepAgentResponsibles(ctx, tx, stepID, current.WorkspaceID, *update.ResponsibleAgentIDs, userID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return domain.ProjectStep{}, err
	}
	return r.getProjectStep(ctx, stepID, userID)
}

func (r *CollabRepository) DeleteProjectStep(ctx context.Context, stepID domain.ProjectStepID, userID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		var workspaceID domain.WorkspaceID
		if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM project_steps WHERE id = ?`, stepID).Scan(&workspaceID); err != nil {
			return mapNotFound(err)
		}
		canManage, err := workspaceProjectAccess(ctx, tx, workspaceID, userID)
		if err != nil {
			return err
		}
		if !canManage {
			return auth.ErrForbidden
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM project_steps WHERE id = ?`, stepID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		return nil
	})
}

func (r *CollabRepository) UpdateRoomProjectStep(ctx context.Context, roomID domain.RoomID, update domain.RoomProjectStepUpdate, userID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		var workspaceID domain.WorkspaceID
		if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM rooms WHERE id = ?`, roomID).Scan(&workspaceID); err != nil {
			return mapNotFound(err)
		}
		canManage, err := workspaceProjectAccess(ctx, tx, workspaceID, userID)
		if err != nil {
			return err
		}
		if !canManage {
			return auth.ErrForbidden
		}
		var value any
		if !update.ClearPlanStep && update.PlanStepID != nil {
			var stepWorkspaceID domain.WorkspaceID
			if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM project_steps WHERE id = ?`, *update.PlanStepID).Scan(&stepWorkspaceID); err != nil {
				return mapNotFound(err)
			}
			if stepWorkspaceID != workspaceID {
				return auth.ErrInvalidInput
			}
			value = *update.PlanStepID
		}
		_, err = tx.ExecContext(ctx, `UPDATE rooms SET plan_step_id = ? WHERE id = ?`, value, roomID)
		return err
	})
}

func (r *CollabRepository) getProjectStep(ctx context.Context, stepID domain.ProjectStepID, userID domain.UserID) (domain.ProjectStep, error) {
	var step domain.ProjectStep
	err := r.store.Read(ctx, func(db *sql.DB) error {
		value, err := scanProjectStep(db.QueryRowContext(ctx, `SELECT ps.id, ps.workspace_id, ps.parent_step_id, ps.title, ps.description, ps.weight, ps.status, ps.sort_order, ps.created_at, ps.updated_at, COUNT(wi.id) FROM project_steps ps LEFT JOIN work_items wi ON wi.plan_step_id = ps.id WHERE ps.id = ? GROUP BY ps.id`, stepID))
		if err != nil {
			return mapNotFound(err)
		}
		if _, err := workspaceProjectAccess(ctx, db, value.WorkspaceID, userID); err != nil {
			return err
		}
		steps := []domain.ProjectStep{value}
		if err := loadProjectStepResponsibles(ctx, db, value.WorkspaceID, steps); err != nil {
			return err
		}
		if err := loadProjectStepWorkspaces(ctx, db, value.WorkspaceID, steps); err != nil {
			return err
		}
		if err := loadProjectStepLocalBindings(ctx, db, value.WorkspaceID, steps); err != nil {
			return err
		}
		value = steps[0]
		step = value
		return nil
	})
	return step, err
}

func loadProjectStepWorkspaces(ctx context.Context, db *sql.DB, workspaceID domain.WorkspaceID, steps []domain.ProjectStep) error {
	if len(steps) == 0 {
		return nil
	}
	byID := make(map[domain.ProjectStepID]*domain.ProjectStep, len(steps))
	for index := range steps {
		byID[steps[index].ID] = &steps[index]
	}
	rows, err := db.QueryContext(ctx, `SELECT psw.step_id, psw.source_name, psw.file_count, psw.total_bytes, psw.revision, psw.last_commit, psw.updated_by_user_id, psw.created_at, psw.updated_at FROM project_step_workspaces psw JOIN project_steps ps ON ps.id = psw.step_id WHERE ps.workspace_id = ?`, workspaceID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value domain.ProjectStepWorkspace
		var userID sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&value.StepID, &value.SourceName, &value.FileCount, &value.TotalBytes, &value.Revision, &value.LastCommit, &userID, &createdAt, &updatedAt); err != nil {
			return err
		}
		if userID.Valid {
			id := domain.UserID(userID.String)
			value.UpdatedByUserID = &id
		}
		var parseErr error
		if value.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, createdAt); parseErr != nil {
			return parseErr
		}
		if value.UpdatedAt, parseErr = time.Parse(time.RFC3339Nano, updatedAt); parseErr != nil {
			return parseErr
		}
		if step := byID[value.StepID]; step != nil {
			step.Workspace = &value
		}
	}
	return rows.Err()
}

func loadProjectStepLocalBindings(ctx context.Context, db *sql.DB, workspaceID domain.WorkspaceID, steps []domain.ProjectStep) error {
	if len(steps) == 0 {
		return nil
	}
	byID := make(map[domain.ProjectStepID]*domain.ProjectStep, len(steps))
	for index := range steps {
		steps[index].LocalBindings = []domain.ProjectStepLocalBinding{}
		byID[steps[index].ID] = &steps[index]
	}
	rows, err := db.QueryContext(ctx, `
SELECT b.step_id, b.user_id, b.runner_id, b.device_id, d.name, b.directory_label,
       b.baseline_commit, r.status, r.last_seen_at, b.created_at, b.updated_at
FROM project_step_local_bindings b
JOIN project_steps ps ON ps.id = b.step_id
JOIN runners r ON r.id = b.runner_id
JOIN devices d ON d.id = b.device_id
WHERE ps.workspace_id = ?
ORDER BY b.created_at, b.user_id, b.runner_id`, workspaceID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value domain.ProjectStepLocalBinding
		var lastSeen sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&value.StepID, &value.UserID, &value.RunnerID, &value.DeviceID, &value.DeviceName,
			&value.DirectoryLabel, &value.BaselineCommit, &value.RunnerStatus, &lastSeen, &createdAt, &updatedAt); err != nil {
			return err
		}
		var parseErr error
		if value.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, createdAt); parseErr != nil {
			return parseErr
		}
		if value.UpdatedAt, parseErr = time.Parse(time.RFC3339Nano, updatedAt); parseErr != nil {
			return parseErr
		}
		if lastSeen.Valid {
			parsed, err := time.Parse(time.RFC3339Nano, lastSeen.String)
			if err != nil {
				return err
			}
			value.LastSeenAt = &parsed
		}
		if step := byID[value.StepID]; step != nil {
			step.LocalBindings = append(step.LocalBindings, value)
		}
	}
	return rows.Err()
}

func (r *CollabRepository) SaveProjectStepLocalBinding(ctx context.Context, stepID domain.ProjectStepID, update domain.ProjectStepLocalBindingUpdate, userID domain.UserID) (domain.ProjectStep, error) {
	if update.RunnerID == "" || update.DeviceID == "" || strings.TrimSpace(update.DirectoryLabel) == "" || len([]rune(strings.TrimSpace(update.DirectoryLabel))) > 160 {
		return domain.ProjectStep{}, auth.ErrInvalidInput
	}
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		var workspaceID domain.WorkspaceID
		if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM project_steps WHERE id = ?`, stepID).Scan(&workspaceID); err != nil {
			return mapNotFound(err)
		}
		var responsible int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_step_user_responsibles WHERE step_id = ? AND user_id = ?`, stepID, userID).Scan(&responsible); err != nil {
			return err
		}
		if responsible == 0 {
			return auth.ErrForbidden
		}
		var deviceID domain.DeviceID
		if err := tx.QueryRowContext(ctx, `
SELECT r.device_id FROM runners r JOIN devices d ON d.id = r.device_id
WHERE r.id = ? AND r.device_id = ? AND d.owner_user_id = ? AND r.status != 'revoked'`,
			update.RunnerID, update.DeviceID, userID).Scan(&deviceID); err != nil {
			return mapNotFound(err)
		}
		baselineCommit := ""
		_ = tx.QueryRowContext(ctx, `SELECT last_commit FROM project_step_workspaces WHERE step_id = ?`, stepID).Scan(&baselineCommit)
		now := time.Now().UTC()
		_, err := tx.ExecContext(ctx, `
INSERT INTO project_step_local_bindings (step_id, user_id, runner_id, device_id, directory_label, baseline_commit, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(step_id, user_id, runner_id) DO UPDATE SET device_id = excluded.device_id,
directory_label = excluded.directory_label, baseline_commit = excluded.baseline_commit, updated_at = excluded.updated_at`,
			stepID, userID, update.RunnerID, update.DeviceID, strings.TrimSpace(update.DirectoryLabel), baselineCommit, formatTime(now), formatTime(now))
		return err
	})
	if err != nil {
		return domain.ProjectStep{}, err
	}
	return r.getProjectStep(ctx, stepID, userID)
}

func (r *CollabRepository) RemoveProjectStepLocalBinding(ctx context.Context, stepID domain.ProjectStepID, runnerID domain.RunnerID, userID domain.UserID) (domain.ProjectStep, error) {
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		var workspaceID domain.WorkspaceID
		if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM project_steps WHERE id = ?`, stepID).Scan(&workspaceID); err != nil {
			return mapNotFound(err)
		}
		if _, err := workspaceProjectAccess(ctx, tx, workspaceID, userID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM project_step_local_bindings WHERE step_id = ? AND runner_id = ? AND user_id = ?`, stepID, runnerID, userID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		return nil
	})
	if err != nil {
		return domain.ProjectStep{}, err
	}
	return r.getProjectStep(ctx, stepID, userID)
}

func (r *CollabRepository) SaveProjectStepWorkspace(ctx context.Context, stepID domain.ProjectStepID, sourceName string, fileCount int, totalBytes int64, commit string, userID domain.UserID) (domain.ProjectStep, error) {
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		var workspaceID domain.WorkspaceID
		if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM project_steps WHERE id = ?`, stepID).Scan(&workspaceID); err != nil {
			return mapNotFound(err)
		}
		canManage, err := workspaceProjectAccess(ctx, tx, workspaceID, userID)
		if err != nil {
			return err
		}
		if !canManage {
			var responsible int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_step_user_responsibles WHERE step_id = ? AND user_id = ?`, stepID, userID).Scan(&responsible); err != nil || responsible == 0 {
				if err != nil {
					return err
				}
				return auth.ErrForbidden
			}
		}
		now := time.Now().UTC()
		_, err = tx.ExecContext(ctx, `INSERT INTO project_step_workspaces (step_id, source_name, file_count, total_bytes, revision, last_commit, updated_by_user_id, created_at, updated_at) VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?) ON CONFLICT(step_id) DO UPDATE SET source_name = excluded.source_name, file_count = excluded.file_count, total_bytes = excluded.total_bytes, revision = project_step_workspaces.revision + 1, last_commit = excluded.last_commit, updated_by_user_id = excluded.updated_by_user_id, updated_at = excluded.updated_at`, stepID, strings.TrimSpace(sourceName), fileCount, totalBytes, strings.TrimSpace(commit), userID, formatTime(now), formatTime(now))
		return err
	})
	if err != nil {
		return domain.ProjectStep{}, err
	}
	return r.getProjectStep(ctx, stepID, userID)
}

func (r *CollabRepository) RemoveProjectStepWorkspace(ctx context.Context, stepID domain.ProjectStepID, userID domain.UserID) (domain.ProjectStep, error) {
	step, err := r.ProjectStepForWorkspaceUpdate(ctx, stepID, userID)
	if err != nil {
		return domain.ProjectStep{}, err
	}
	err = r.store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM project_step_workspaces WHERE step_id = ?`, step.ID)
		return err
	})
	if err != nil {
		return domain.ProjectStep{}, err
	}
	return r.getProjectStep(ctx, stepID, userID)
}

func (r *CollabRepository) ProjectStepForWorkspaceUpdate(ctx context.Context, stepID domain.ProjectStepID, userID domain.UserID) (domain.ProjectStep, error) {
	step, err := r.getProjectStep(ctx, stepID, userID)
	if err != nil {
		return domain.ProjectStep{}, err
	}
	if step.WorkspaceID == "" {
		return domain.ProjectStep{}, auth.ErrNotFound
	}
	canManage := false
	err = r.store.Read(ctx, func(db *sql.DB) error {
		var role domain.WorkspaceRole
		if err := db.QueryRowContext(ctx, `SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, step.WorkspaceID, userID).Scan(&role); err != nil {
			return mapNotFound(err)
		}
		canManage = role == domain.WorkspaceOwner || role == domain.WorkspaceAdmin
		if canManage {
			return nil
		}
		var responsible int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_step_user_responsibles WHERE step_id = ? AND user_id = ?`, stepID, userID).Scan(&responsible); err != nil {
			return err
		}
		if responsible == 0 {
			return auth.ErrForbidden
		}
		return nil
	})
	return step, err
}

// ProjectStepForWorkspaceRead intentionally uses workspace membership as the
// complete read boundary. Shared project drafts are team material: every
// workspace member can inspect the current immutable snapshot, while upload,
// unlink, review and merge continue to use their separate write checks.
func (r *CollabRepository) ProjectStepForWorkspaceRead(ctx context.Context, stepID domain.ProjectStepID, userID domain.UserID) (domain.ProjectStep, error) {
	return r.getProjectStep(ctx, stepID, userID)
}

func scanProjectStep(row rowScanner) (domain.ProjectStep, error) {
	step := domain.ProjectStep{ResponsibleUserIDs: []domain.UserID{}, ResponsibleAgentIDs: []domain.AgentID{}}
	var parentID sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(&step.ID, &step.WorkspaceID, &parentID, &step.Title, &step.Description, &step.Weight, &step.Status, &step.SortOrder, &createdAt, &updatedAt, &step.LinkedItemCount); err != nil {
		return step, err
	}
	if parentID.Valid {
		value := domain.ProjectStepID(parentID.String)
		step.ParentStepID = &value
	}
	var err error
	if step.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return step, fmt.Errorf("parse project step created_at: %w", err)
	}
	if step.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return step, fmt.Errorf("parse project step updated_at: %w", err)
	}
	return step, nil
}

func replaceProjectStepResponsibles(ctx context.Context, tx *sql.Tx, stepID domain.ProjectStepID, workspaceID domain.WorkspaceID, userIDs []domain.UserID, agentIDs []domain.AgentID, assignedBy domain.UserID) error {
	if err := replaceProjectStepUserResponsibles(ctx, tx, stepID, workspaceID, userIDs, assignedBy); err != nil {
		return err
	}
	return replaceProjectStepAgentResponsibles(ctx, tx, stepID, workspaceID, agentIDs, assignedBy)
}

func replaceProjectStepUserResponsibles(ctx context.Context, tx *sql.Tx, stepID domain.ProjectStepID, workspaceID domain.WorkspaceID, userIDs []domain.UserID, assignedBy domain.UserID) error {
	unique := make(map[domain.UserID]bool, len(userIDs))
	for _, id := range userIDs {
		if id == "" || unique[id] {
			continue
		}
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, id).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return auth.ErrInvalidInput
		}
		unique[id] = true
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_step_user_responsibles WHERE step_id = ?`, stepID); err != nil {
		return err
	}
	for id := range unique {
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_step_user_responsibles (step_id, user_id, assigned_by_user_id) VALUES (?, ?, ?)`, stepID, id, assignedBy); err != nil {
			return err
		}
	}
	return nil
}

func replaceProjectStepAgentResponsibles(ctx context.Context, tx *sql.Tx, stepID domain.ProjectStepID, workspaceID domain.WorkspaceID, agentIDs []domain.AgentID, assignedBy domain.UserID) error {
	unique := make(map[domain.AgentID]bool, len(agentIDs))
	for _, id := range agentIDs {
		if id == "" || unique[id] {
			continue
		}
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agents WHERE workspace_id = ? AND id = ?`, workspaceID, id).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return auth.ErrInvalidInput
		}
		unique[id] = true
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_step_agent_responsibles WHERE step_id = ?`, stepID); err != nil {
		return err
	}
	for id := range unique {
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_step_agent_responsibles (step_id, agent_id, assigned_by_user_id) VALUES (?, ?, ?)`, stepID, id, assignedBy); err != nil {
			return err
		}
	}
	return nil
}

func loadProjectStepResponsibles(ctx context.Context, db *sql.DB, workspaceID domain.WorkspaceID, steps []domain.ProjectStep) error {
	indexes := make(map[domain.ProjectStepID]int, len(steps))
	for index := range steps {
		steps[index].ResponsibleUserIDs = []domain.UserID{}
		steps[index].ResponsibleAgentIDs = []domain.AgentID{}
		indexes[steps[index].ID] = index
	}
	userRows, err := db.QueryContext(ctx, `SELECT r.step_id, r.user_id FROM project_step_user_responsibles r JOIN project_steps ps ON ps.id = r.step_id WHERE ps.workspace_id = ? ORDER BY r.assigned_at, r.user_id`, workspaceID)
	if err != nil {
		return err
	}
	for userRows.Next() {
		var stepID domain.ProjectStepID
		var userID domain.UserID
		if err := userRows.Scan(&stepID, &userID); err != nil {
			userRows.Close()
			return err
		}
		if index, ok := indexes[stepID]; ok {
			steps[index].ResponsibleUserIDs = append(steps[index].ResponsibleUserIDs, userID)
		}
	}
	if err := userRows.Err(); err != nil {
		userRows.Close()
		return err
	}
	if err := userRows.Close(); err != nil {
		return err
	}
	agentRows, err := db.QueryContext(ctx, `SELECT r.step_id, r.agent_id FROM project_step_agent_responsibles r JOIN project_steps ps ON ps.id = r.step_id WHERE ps.workspace_id = ? ORDER BY r.assigned_at, r.agent_id`, workspaceID)
	if err != nil {
		return err
	}
	defer agentRows.Close()
	for agentRows.Next() {
		var stepID domain.ProjectStepID
		var agentID domain.AgentID
		if err := agentRows.Scan(&stepID, &agentID); err != nil {
			return err
		}
		if index, ok := indexes[stepID]; ok {
			steps[index].ResponsibleAgentIDs = append(steps[index].ResponsibleAgentIDs, agentID)
		}
	}
	return agentRows.Err()
}

func calculateProjectProgress(steps []domain.ProjectStep) int {
	byParent := make(map[string][]int)
	for index := range steps {
		parent := ""
		if steps[index].ParentStepID != nil {
			parent = string(*steps[index].ParentStepID)
		}
		byParent[parent] = append(byParent[parent], index)
	}
	visiting := make(map[domain.ProjectStepID]bool)
	var progress func(int) int
	progress = func(index int) int {
		step := &steps[index]
		if visiting[step.ID] {
			return 0
		}
		visiting[step.ID] = true
		defer delete(visiting, step.ID)
		children := byParent[string(step.ID)]
		weighted, totalWeight := 0, 0
		for _, childIndex := range children {
			child := &steps[childIndex]
			if child.Status == domain.ProjectStepCancelled {
				child.Progress = 0
				continue
			}
			child.Progress = progress(childIndex)
			weighted += child.Progress * child.Weight
			totalWeight += child.Weight
		}
		if totalWeight > 0 {
			step.Progress = weighted / totalWeight
		} else if step.Status == domain.ProjectStepDone {
			step.Progress = 100
		} else {
			step.Progress = 0
		}
		return step.Progress
	}
	weighted, totalWeight := 0, 0
	for _, index := range byParent[""] {
		step := &steps[index]
		if step.Status == domain.ProjectStepCancelled {
			step.Progress = 0
			continue
		}
		step.Progress = progress(index)
		weighted += step.Progress * step.Weight
		totalWeight += step.Weight
	}
	if totalWeight == 0 {
		return 0
	}
	return weighted / totalWeight
}
