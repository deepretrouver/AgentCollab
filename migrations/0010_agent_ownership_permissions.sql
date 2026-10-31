ALTER TABLE devices ADD COLUMN stable_key TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_devices_owner_stable_key
ON devices(workspace_id, owner_user_id, stable_key)
WHERE stable_key != '' AND status != 'revoked';

CREATE TABLE agent_access_policies (
    agent_id TEXT PRIMARY KEY REFERENCES agents(id) ON DELETE CASCADE,
    visibility TEXT NOT NULL DEFAULT 'workspace' CHECK (visibility IN ('private', 'workspace')),
    invocation_scope TEXT NOT NULL DEFAULT 'owner_only' CHECK (invocation_scope IN ('owner_only', 'room_members')),
    shared_skills_json TEXT NOT NULL DEFAULT '[]',
    archived INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),
    updated_by_user_id TEXT NOT NULL REFERENCES users(id),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO agent_access_policies (agent_id, updated_by_user_id)
SELECT id, owner_user_id FROM agents;

CREATE INDEX idx_agent_access_visible
ON agent_access_policies(visibility, invocation_scope, archived, agent_id);
