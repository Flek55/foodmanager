-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, user_agent, expires_at)
VALUES (@token_hash, @user_id, @user_agent, @expires_at);

-- name: GetActiveSession :one
SELECT sqlc.embed(u), s.last_seen_at, s.expires_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = @token_hash AND s.expires_at > now();

-- name: ExtendSession :exec
UPDATE sessions
SET last_seen_at = now(), expires_at = @expires_at
WHERE token_hash = @token_hash;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = @token_hash;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= now();
