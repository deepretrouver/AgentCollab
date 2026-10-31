UPDATE agents
SET source_display_name = (
      SELECT COALESCE(NULLIF(users.display_name, ''), users.username)
      FROM users
      WHERE users.id = agents.owner_user_id
    ) || ' ' || CASE lower(type)
      WHEN 'hermes' THEN 'Hermes'
      WHEN 'codex' THEN 'Codex'
      WHEN 'workbuddy' THEN 'WorkBuddy'
      WHEN 'claude-code' THEN 'Claude Code'
      WHEN 'opencode' THEN 'OpenCode'
      WHEN 'openclaw' THEN 'OpenClaw'
      WHEN 'codex-acp' THEN 'Codex ACP'
      WHEN 'claude-code-acp' THEN 'Claude Code ACP'
      ELSE type || ' Agent'
    END,
    display_name = (
      SELECT COALESCE(NULLIF(users.display_name, ''), users.username)
      FROM users
      WHERE users.id = agents.owner_user_id
    ) || ' ' || CASE lower(type)
      WHEN 'hermes' THEN 'Hermes'
      WHEN 'codex' THEN 'Codex'
      WHEN 'workbuddy' THEN 'WorkBuddy'
      WHEN 'claude-code' THEN 'Claude Code'
      WHEN 'opencode' THEN 'OpenCode'
      WHEN 'openclaw' THEN 'OpenClaw'
      WHEN 'codex-acp' THEN 'Codex ACP'
      WHEN 'claude-code-acp' THEN 'Claude Code ACP'
      ELSE type || ' Agent'
    END
WHERE local_key LIKE 'scan-%';
