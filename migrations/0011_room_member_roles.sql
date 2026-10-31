ALTER TABLE room_members
ADD COLUMN role TEXT NOT NULL DEFAULT 'member'
CHECK (role IN ('owner', 'admin', 'member'));

-- Preserve existing rooms by assigning one deterministic owner. Prefer the
-- workspace owner when present; otherwise use the earliest room member.
UPDATE room_members
SET role = 'owner'
WHERE user_id = (
    SELECT candidate.user_id
    FROM room_members candidate
    JOIN rooms ON rooms.id = candidate.room_id
    JOIN workspaces ON workspaces.id = rooms.workspace_id
    WHERE candidate.room_id = room_members.room_id
    ORDER BY
        CASE WHEN candidate.user_id = workspaces.owner_user_id THEN 0 ELSE 1 END,
        candidate.joined_at,
        candidate.user_id
    LIMIT 1
);

CREATE INDEX idx_room_members_role ON room_members(room_id, role);
