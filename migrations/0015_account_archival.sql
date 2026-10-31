-- Account removal from the Hub directory is an archival operation.  Keeping
-- the user row preserves historic message attribution and foreign-key audits,
-- while archived accounts cannot authenticate or appear in the active list.
ALTER TABLE users ADD COLUMN archived_at TEXT;

CREATE INDEX idx_users_active_directory
ON users(role, display_name, id)
WHERE archived_at IS NULL;
