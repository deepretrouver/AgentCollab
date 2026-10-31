CREATE TABLE room_agents (
    room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    added_by_user_id TEXT NOT NULL REFERENCES users(id),
    added_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (room_id, agent_id)
);

CREATE INDEX idx_room_agents_agent ON room_agents(agent_id, room_id);

ALTER TABLE agents ADD COLUMN imported INTEGER NOT NULL DEFAULT 0 CHECK (imported IN (0, 1));
ALTER TABLE task_attempts ADD COLUMN execution_receipt_json TEXT NOT NULL DEFAULT '{}';
