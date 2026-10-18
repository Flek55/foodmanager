-- name: CreateInvite :one
INSERT INTO invites (code, household_id, created_by, max_uses, expires_at)
VALUES (@code, @household_id, @created_by, @max_uses, @expires_at)
RETURNING *;

-- name: ListActiveInvites :many
SELECT i.*, u.display_name AS created_by_name
FROM invites i
JOIN users u ON u.id = i.created_by
WHERE i.household_id = @household_id
  AND i.revoked_at IS NULL
  AND i.expires_at > now()
  AND i.uses < i.max_uses
ORDER BY i.created_at DESC;

-- name: RevokeInvite :execrows
UPDATE invites SET revoked_at = now()
WHERE household_id = @household_id AND code = @code AND revoked_at IS NULL;

-- name: GetActiveInviteForUpdate :one
SELECT * FROM invites
WHERE code = @code
  AND revoked_at IS NULL
  AND expires_at > now()
  AND uses < max_uses
FOR UPDATE;

-- name: IncrementInviteUses :exec
UPDATE invites SET uses = uses + 1 WHERE code = @code;

-- name: GetInvitePreview :one
SELECT
    h.name AS household_name,
    u.display_name AS invited_by,
    (SELECT count(*) FROM memberships m WHERE m.household_id = h.id)::int AS member_count
FROM invites i
JOIN households h ON h.id = i.household_id
JOIN users u ON u.id = i.created_by
WHERE i.code = @code
  AND i.revoked_at IS NULL
  AND i.expires_at > now()
  AND i.uses < i.max_uses;
