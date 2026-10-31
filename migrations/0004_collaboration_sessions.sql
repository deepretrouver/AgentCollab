ALTER TABLE messages ADD COLUMN mention_agent_ids_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE messages ADD COLUMN reply_to_message_id TEXT REFERENCES messages(message_id);
ALTER TABLE messages ADD COLUMN root_message_id TEXT REFERENCES messages(message_id);
ALTER TABLE messages ADD COLUMN metadata_json TEXT NOT NULL DEFAULT '{}';

ALTER TABLE tasks ADD COLUMN root_message_id TEXT REFERENCES messages(message_id);
ALTER TABLE tasks ADD COLUMN source_message_id TEXT REFERENCES messages(message_id);
ALTER TABLE tasks ADD COLUMN collaboration_depth INTEGER NOT NULL DEFAULT 0 CHECK (collaboration_depth >= 0);

CREATE TABLE room_agent_settings (
    room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    response_mode TEXT NOT NULL DEFAULT 'mention' CHECK (response_mode IN ('mention', 'coordinator', 'human_all', 'brainstorm', 'muted')),
    reply_delay_seconds INTEGER NOT NULL DEFAULT 0 CHECK (reply_delay_seconds BETWEEN 0 AND 60),
    role_prompt TEXT NOT NULL DEFAULT '',
    max_auto_turns INTEGER NOT NULL DEFAULT 4 CHECK (max_auto_turns BETWEEN 1 AND 8),
    updated_by_user_id TEXT NOT NULL REFERENCES users(id),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (room_id, agent_id)
);

INSERT INTO room_agent_settings (room_id, agent_id, updated_by_user_id)
SELECT room_id, agent_id, added_by_user_id FROM room_agents;

CREATE TABLE agent_sessions (
    room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    native_session_id TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'archived', 'failed')),
    last_task_id TEXT REFERENCES tasks(id),
    last_message_sequence INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (room_id, agent_id)
);

CREATE UNIQUE INDEX idx_tasks_routed_once
ON tasks(root_message_id, source_message_id, target_agent_id)
WHERE root_message_id IS NOT NULL AND source_message_id IS NOT NULL;

CREATE INDEX idx_tasks_root_depth ON tasks(root_message_id, collaboration_depth, created_at);
CREATE INDEX idx_agent_sessions_agent ON agent_sessions(agent_id, state);
