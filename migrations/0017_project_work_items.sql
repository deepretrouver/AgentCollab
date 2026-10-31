-- A room is also a lightweight project cockpit. Work items are the
-- human-facing units shown in its overview and board; individual Agent tasks
-- remain execution records and can fan out beneath one item.
CREATE TABLE work_items (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    root_message_id TEXT REFERENCES messages(message_id) ON DELETE SET NULL,
    kind TEXT NOT NULL DEFAULT 'task' CHECK (kind IN ('task', 'discussion', 'brainstorm')),
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'in_progress' CHECK (status IN ('backlog', 'ready', 'in_progress', 'review', 'done', 'cancelled')),
    priority TEXT NOT NULL DEFAULT 'normal' CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    owner_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    due_at TEXT,
    accepted_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (room_id, root_message_id)
);

ALTER TABLE tasks ADD COLUMN work_item_id TEXT REFERENCES work_items(id) ON DELETE SET NULL;

CREATE INDEX idx_work_items_room_status ON work_items(room_id, status, updated_at);
CREATE INDEX idx_work_items_owner_status ON work_items(owner_user_id, status, due_at);
CREATE INDEX idx_tasks_work_item ON tasks(work_item_id, state, created_at);

-- Preserve existing conversations by grouping every historical task tree under
-- its visible human root. A delivery is intentionally only "review": a human
-- must explicitly accept it before the work item becomes done.
INSERT INTO work_items (
    id, workspace_id, room_id, root_message_id, kind, title, description,
    status, priority, owner_user_id, created_at, updated_at
)
SELECT
    'work_' || lower(hex(randomblob(12))),
    t.workspace_id,
    t.room_id,
    t.root_message_id,
    CASE WHEN lower(trim(m.body)) LIKE '/brainstorm%' THEN 'brainstorm' ELSE 'task' END,
    COALESCE(
        NULLIF(json_extract(m.metadata_json, '$.task_title'), ''),
        NULLIF(substr(trim(m.body), 1, 120), ''),
        MIN(t.title)
    ),
    COALESCE(m.body, ''),
    CASE
        WHEN SUM(CASE WHEN t.state IN ('queued', 'dispatched', 'running') THEN 1 ELSE 0 END) > 0 THEN 'in_progress'
        WHEN SUM(CASE WHEN t.state = 'delivered' THEN 1 ELSE 0 END) > 0 THEN 'review'
        WHEN SUM(CASE WHEN t.state = 'cancelled' THEN 1 ELSE 0 END) = COUNT(*) THEN 'cancelled'
        ELSE 'in_progress'
    END,
    'normal',
    CASE WHEN m.sender_kind = 'user' THEN m.sender_id ELSE NULL END,
    MIN(t.created_at),
    MAX(t.updated_at)
FROM tasks t
JOIN messages m ON m.message_id = t.root_message_id
WHERE t.root_message_id IS NOT NULL
GROUP BY t.workspace_id, t.room_id, t.root_message_id;

UPDATE tasks
SET work_item_id = (
    SELECT wi.id FROM work_items wi
    WHERE wi.room_id = tasks.room_id AND wi.root_message_id = tasks.root_message_id
)
WHERE root_message_id IS NOT NULL;
