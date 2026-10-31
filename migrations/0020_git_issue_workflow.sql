ALTER TABLE work_items ADD COLUMN git_branch TEXT NOT NULL DEFAULT '';
ALTER TABLE work_items ADD COLUMN merge_status TEXT NOT NULL DEFAULT 'none'
    CHECK (merge_status IN ('none', 'open', 'merged', 'conflict'));
ALTER TABLE work_items ADD COLUMN merged_commit TEXT NOT NULL DEFAULT '';

UPDATE work_items
SET git_branch = 'issue/' || root_message_id,
    merge_status = 'open'
WHERE plan_step_id IS NOT NULL AND root_message_id IS NOT NULL AND root_message_id <> '';

CREATE INDEX idx_work_items_git_branch ON work_items(workspace_id, git_branch, merge_status);
