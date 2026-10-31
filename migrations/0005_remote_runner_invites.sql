CREATE TABLE runner_invites (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_by_user_id TEXT NOT NULL REFERENCES users(id),
    code_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    used_by_runner_id TEXT REFERENCES runners(id),
    used_at TEXT,
    revoked_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX runner_invites_active_idx
ON runner_invites(code_hash, expires_at)
WHERE used_at IS NULL AND revoked_at IS NULL;

-- `brainstorm` used the same routing behavior as `human_all`. Keep old
-- databases honest by folding the legacy label into the supported mode.
UPDATE room_agent_settings
SET response_mode = 'human_all'
WHERE response_mode = 'brainstorm';
