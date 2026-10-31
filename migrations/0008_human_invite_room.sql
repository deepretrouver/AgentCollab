-- A human invitation can target one room as well as its parent workspace.
-- Accepting it grants the person both memberships atomically.
ALTER TABLE invites ADD COLUMN room_id TEXT REFERENCES rooms(id) ON DELETE SET NULL;

CREATE INDEX invites_active_room_idx
ON invites(room_id, expires_at)
WHERE used_at IS NULL AND revoked_at IS NULL;
