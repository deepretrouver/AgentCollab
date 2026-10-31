-- Freeze the canonical project-step snapshot used by each claimed task. The
-- value is assigned when the task is first claimed so every retry executes
-- against the same immutable Git commit, including on a different Runner.
ALTER TABLE tasks ADD COLUMN project_workspace_commit TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN project_workspace_source_name TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN project_workspace_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN project_workspace_file_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN project_workspace_total_bytes INTEGER NOT NULL DEFAULT 0;
