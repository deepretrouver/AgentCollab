-- A project step owns one canonical directory inside the protected workspace
-- repository. Contributors upload a local directory snapshot through the UI;
-- AgentCollab records the resulting main-branch commit here.
CREATE TABLE project_step_workspaces (
    step_id TEXT PRIMARY KEY REFERENCES project_steps(id) ON DELETE CASCADE,
    source_name TEXT NOT NULL DEFAULT '',
    file_count INTEGER NOT NULL DEFAULT 0,
    total_bytes INTEGER NOT NULL DEFAULT 0,
    revision INTEGER NOT NULL DEFAULT 1,
    last_commit TEXT NOT NULL DEFAULT '',
    updated_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

ALTER TABLE work_items ADD COLUMN branch_commit TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_project_step_workspaces_updated ON project_step_workspaces(updated_at);
