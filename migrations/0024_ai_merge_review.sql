-- AI-assisted merge review keeps the human administrator responsible for the
-- decision while delegating technical inspection and post-merge validation to
-- explicitly selected room Agents.
ALTER TABLE work_items ADD COLUMN review_agent_id TEXT REFERENCES agents(id) ON DELETE SET NULL;
ALTER TABLE work_items ADD COLUMN verification_agent_id TEXT REFERENCES agents(id) ON DELETE SET NULL;
ALTER TABLE work_items ADD COLUMN review_task_id TEXT REFERENCES tasks(id) ON DELETE SET NULL;
ALTER TABLE work_items ADD COLUMN verification_task_id TEXT REFERENCES tasks(id) ON DELETE SET NULL;
ALTER TABLE work_items ADD COLUMN ai_review_status TEXT NOT NULL DEFAULT 'none'
    CHECK (ai_review_status IN ('none', 'pending', 'passed', 'changes_requested', 'failed'));
ALTER TABLE work_items ADD COLUMN ai_review_summary TEXT NOT NULL DEFAULT '';
ALTER TABLE work_items ADD COLUMN verification_status TEXT NOT NULL DEFAULT 'none'
    CHECK (verification_status IN ('none', 'pending', 'passed', 'failed'));
ALTER TABLE work_items ADD COLUMN verification_summary TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN workflow_role TEXT NOT NULL DEFAULT ''
    CHECK (workflow_role IN ('', 'ai_merge_review', 'post_merge_verification'));

CREATE INDEX idx_work_items_ai_review ON work_items(room_id, ai_review_status, verification_status, updated_at);
