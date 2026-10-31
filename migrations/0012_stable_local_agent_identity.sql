ALTER TABLE agents ADD COLUMN local_key TEXT NOT NULL DEFAULT '';

-- Existing Runner releases did not persist the local catalog identifier. It is
-- safe to infer it only when a Runner has exactly one Agent of that type.
UPDATE agents
SET local_key = 'scan-' || type
WHERE local_key = ''
  AND 1 = (
    SELECT COUNT(*)
    FROM agents siblings
    WHERE siblings.runner_id = agents.runner_id
      AND siblings.type = agents.type
  );

CREATE UNIQUE INDEX idx_agents_runner_local_key
ON agents(runner_id, local_key)
WHERE local_key != '';
