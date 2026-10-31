-- Keep a Runner's historical server Agent IDs resolvable after an Agent is
-- imported into another workspace and its original workspace is deleted.
-- The ID belongs to the Runner credential file, so it cannot cascade with the
-- old Agent row.
CREATE TABLE runner_agent_aliases (
    runner_id TEXT NOT NULL REFERENCES runners(id) ON DELETE CASCADE,
    legacy_agent_id TEXT NOT NULL,
    runtime_local_key TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (runner_id, legacy_agent_id)
);

CREATE INDEX idx_runner_agent_aliases_runtime
ON runner_agent_aliases (runner_id, runtime_local_key);
