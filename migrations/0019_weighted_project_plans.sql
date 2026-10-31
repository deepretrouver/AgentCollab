-- Project progress is defined by an explicit, weighted plan. Chat topics and
-- Agent executions remain evidence beneath a step, but never create progress
-- unless a room member deliberately assigns them to that step and a manager
-- accepts the step as done.
CREATE TABLE project_plans (
    workspace_id TEXT PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    objective TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'planning' CHECK (status IN ('planning', 'active', 'completed', 'cancelled')),
    created_by_user_id TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE project_steps (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    parent_step_id TEXT REFERENCES project_steps(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    weight INTEGER NOT NULL DEFAULT 100 CHECK (weight >= 1 AND weight <= 1000),
    status TEXT NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'in_progress', 'review', 'done', 'cancelled')),
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_by_user_id TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

ALTER TABLE work_items ADD COLUMN plan_step_id TEXT REFERENCES project_steps(id) ON DELETE SET NULL;
ALTER TABLE rooms ADD COLUMN plan_step_id TEXT REFERENCES project_steps(id) ON DELETE SET NULL;

CREATE INDEX idx_project_steps_workspace_parent_order ON project_steps(workspace_id, parent_step_id, sort_order, created_at);
CREATE INDEX idx_project_steps_workspace_status ON project_steps(workspace_id, status, updated_at);
CREATE INDEX idx_work_items_plan_step ON work_items(plan_step_id, status, updated_at);
CREATE INDEX idx_rooms_plan_step ON rooms(plan_step_id, created_at);
