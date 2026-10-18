-- name: CreateHousehold :one
INSERT INTO households (name, created_by)
VALUES (@name, @created_by)
RETURNING *;

-- name: RenameHousehold :exec
UPDATE households SET name = @name, updated_at = now() WHERE id = @id;

-- name: DeleteHousehold :exec
DELETE FROM households WHERE id = @id;

-- Serializes membership changes of one household inside a transaction.
-- name: LockHousehold :one
SELECT id FROM households WHERE id = @id FOR UPDATE;

-- Returns the household only if the user is a member of it.
-- name: GetHouseholdForMember :one
SELECT h.id, h.name, m.role
FROM households h
JOIN memberships m ON m.household_id = h.id
WHERE h.id = @household_id AND m.user_id = @user_id;

-- name: ListUserHouseholds :many
SELECT h.id, h.name, m.role
FROM memberships m
JOIN households h ON h.id = m.household_id
WHERE m.user_id = @user_id
ORDER BY m.joined_at;

-- name: AddMember :execrows
INSERT INTO memberships (household_id, user_id, role)
VALUES (@household_id, @user_id, @role)
ON CONFLICT (household_id, user_id) DO NOTHING;

-- name: RemoveMember :execrows
DELETE FROM memberships WHERE household_id = @household_id AND user_id = @user_id;

-- name: GetMemberRole :one
SELECT role FROM memberships WHERE household_id = @household_id AND user_id = @user_id;

-- name: ListMembers :many
SELECT u.id AS user_id, u.display_name, u.avatar_url, m.role, m.joined_at
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.household_id = @household_id
ORDER BY m.joined_at, u.id;

-- name: CountMembers :one
SELECT
    count(*)::int AS members,
    (count(*) FILTER (WHERE role = 'owner'))::int AS owners
FROM memberships
WHERE household_id = @household_id;

-- Promotes the longest-standing member, used when the last owner leaves.
-- name: PromoteOldestMember :exec
UPDATE memberships AS target SET role = 'owner'
WHERE target.household_id = @household_id
  AND target.user_id = (
      SELECT oldest.user_id FROM memberships AS oldest
      WHERE oldest.household_id = @household_id
      ORDER BY oldest.joined_at, oldest.user_id
      LIMIT 1
  );
