-- A Runner is a physical client.  Its Agent can be made available in more
-- than one workspace without pairing the same computer again.  `local_key`
-- remains unique per Runner for the stored workspace record, while
-- `runtime_local_key` always identifies the real local CLI used by Runner.
ALTER TABLE agents ADD COLUMN runtime_local_key TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN source_display_name TEXT NOT NULL DEFAULT '';

UPDATE agents
SET runtime_local_key = local_key
WHERE runtime_local_key = '';

UPDATE agents
SET source_display_name = display_name
WHERE source_display_name = '';

CREATE INDEX idx_agents_runner_runtime_local_key
ON agents(runner_id, runtime_local_key)
WHERE runtime_local_key != '';
