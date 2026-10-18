-- ---- Accounts ----

-- name: UpsertTelegramAccount :one
INSERT INTO telegram_accounts (user_id, telegram_user_id, chat_id, username, first_name)
VALUES (@user_id, @telegram_user_id, @chat_id, sqlc.narg(username), sqlc.narg(first_name))
ON CONFLICT (user_id) DO UPDATE
SET telegram_user_id = EXCLUDED.telegram_user_id, chat_id = EXCLUDED.chat_id,
    username = EXCLUDED.username, first_name = EXCLUDED.first_name,
    blocked_at = NULL, linked_at = now()
RETURNING *;

-- name: GetTelegramAccount :one
SELECT * FROM telegram_accounts WHERE user_id = @user_id;

-- name: GetTelegramAccountByTelegramID :one
SELECT * FROM telegram_accounts WHERE telegram_user_id = @telegram_user_id;

-- name: DeleteTelegramAccount :execrows
DELETE FROM telegram_accounts WHERE user_id = @user_id;

-- name: UpdateTelegramSettings :one
UPDATE telegram_accounts
SET notify_added = @notify_added, notify_stock = @notify_stock
WHERE user_id = @user_id
RETURNING *;

-- name: SetTelegramDefaultHousehold :exec
UPDATE telegram_accounts SET default_household_id = @household_id WHERE user_id = @user_id;

-- name: MarkTelegramBlocked :exec
UPDATE telegram_accounts SET blocked_at = now() WHERE chat_id = @chat_id;

-- Members of a household who get notifications from the bot.
-- name: ListTelegramRecipients :many
SELECT t.user_id, t.chat_id, t.notify_added, t.notify_stock
FROM memberships AS m
JOIN telegram_accounts AS t ON t.user_id = m.user_id
WHERE m.household_id = @household_id AND t.blocked_at IS NULL;

-- ---- One-time deep-link tokens ----

-- name: CreateTelegramToken :exec
INSERT INTO telegram_tokens (token_hash, purpose, user_id, user_agent, expires_at)
VALUES (@token_hash, @purpose, sqlc.narg(user_id), sqlc.narg(user_agent), @expires_at);

-- Locks the token row: approving/consuming it must happen exactly once.
-- name: GetTelegramTokenForUpdate :one
SELECT * FROM telegram_tokens
WHERE token_hash = @token_hash AND purpose = @purpose
FOR UPDATE;

-- name: ApproveTelegramToken :exec
UPDATE telegram_tokens
SET user_id = @user_id, telegram_user_id = @telegram_user_id, approved_at = now()
WHERE token_hash = @token_hash;

-- name: RejectTelegramToken :exec
UPDATE telegram_tokens SET rejected_at = now(), telegram_user_id = @telegram_user_id
WHERE token_hash = @token_hash;

-- name: UseTelegramToken :exec
UPDATE telegram_tokens SET used_at = now() WHERE token_hash = @token_hash;

-- name: DeleteExpiredTelegramTokens :execrows
DELETE FROM telegram_tokens WHERE expires_at < now() - interval '1 day';

-- ---- Users created from Telegram ----

-- name: CreateTelegramUser :one
INSERT INTO users (display_name) VALUES (@display_name) RETURNING *;

-- name: GetHouseholdName :one
SELECT name FROM households WHERE id = @id;

-- The user wrote to the bot again: deliver notifications again.
-- name: UnblockTelegram :exec
UPDATE telegram_accounts SET blocked_at = NULL WHERE telegram_user_id = @telegram_user_id;
