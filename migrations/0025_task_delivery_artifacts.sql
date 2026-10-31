-- Runner-created files are uploaded before task completion and attached to the
-- final visible delivery message when the Hub commits that delivery.
ALTER TABLE message_attachments ADD COLUMN task_id TEXT REFERENCES tasks(id) ON DELETE CASCADE;

CREATE INDEX idx_message_attachments_task ON message_attachments(task_id, created_at, id);
