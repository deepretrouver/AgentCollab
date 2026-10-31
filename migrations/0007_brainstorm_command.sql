UPDATE room_agent_settings
SET response_mode = 'human_all',
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE response_mode = 'brainstorm';
