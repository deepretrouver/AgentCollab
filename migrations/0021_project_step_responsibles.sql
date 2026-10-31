-- A project step can have multiple responsible humans and Agents. Assigning or
-- removing responsibility never changes workspace membership, Agent access, or
-- historical work-item records.
CREATE TABLE project_step_user_responsibles (
    step_id TEXT NOT NULL REFERENCES project_steps(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    assigned_by_user_id TEXT NOT NULL REFERENCES users(id),
    assigned_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (step_id, user_id)
);

CREATE TABLE project_step_agent_responsibles (
    step_id TEXT NOT NULL REFERENCES project_steps(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    assigned_by_user_id TEXT NOT NULL REFERENCES users(id),
    assigned_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (step_id, agent_id)
);

CREATE INDEX idx_project_step_user_responsibles_user ON project_step_user_responsibles(user_id, step_id);
CREATE INDEX idx_project_step_agent_responsibles_agent ON project_step_agent_responsibles(agent_id, step_id);
