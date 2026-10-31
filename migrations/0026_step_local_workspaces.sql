-- A responsible member may bind a real directory on their own Client to one
-- project step. The absolute local path is deliberately never stored here.
CREATE TABLE project_step_local_bindings (
    step_id TEXT NOT NULL REFERENCES project_steps(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    runner_id TEXT NOT NULL REFERENCES runners(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    directory_label TEXT NOT NULL,
    baseline_commit TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (step_id, user_id, runner_id)
);

CREATE INDEX idx_project_step_local_bindings_runner ON project_step_local_bindings(runner_id, step_id);

-- The execution source is explicit so a progress query can run in the
-- responsible member's live directory without pretending that it is a Git
-- submission. Existing step tasks keep their historical Server-snapshot rule.
ALTER TABLE tasks ADD COLUMN execution_source TEXT NOT NULL DEFAULT ''
    CHECK (execution_source IN ('', 'official', 'responsible_local', 'none'));
