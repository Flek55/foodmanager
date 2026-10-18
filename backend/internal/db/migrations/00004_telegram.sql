-- Telegram: linked accounts (notifications, bot, login) and one-time tokens
-- for deep links (t.me/<bot>?start=<token>).

-- +goose Up
CREATE TABLE telegram_accounts (
    user_id              uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    telegram_user_id     bigint      NOT NULL UNIQUE,
    -- For private chats this equals telegram_user_id, but we store what
    -- Telegram told us rather than rely on that.
    chat_id              bigint      NOT NULL,
    username             text,
    first_name           text,
    -- Household the bot adds to when the user is in several.
    default_household_id uuid        REFERENCES households (id) ON DELETE SET NULL,
    notify_added         boolean     NOT NULL DEFAULT true,
    notify_stock         boolean     NOT NULL DEFAULT true,
    -- Set when Telegram says the user blocked the bot: stop sending.
    blocked_at           timestamptz,
    linked_at            timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE telegram_tokens (
    -- SHA-256 of the token; the token itself only travels in the deep link.
    token_hash       bytea PRIMARY KEY,
    purpose          text        NOT NULL CHECK (purpose IN ('link', 'login')),
    -- link: the user who asked to link. login: the user who approved.
    user_id          uuid        REFERENCES users (id) ON DELETE CASCADE,
    telegram_user_id bigint,
    approved_at      timestamptz,
    rejected_at      timestamptz,
    used_at          timestamptz,
    user_agent       text,
    expires_at       timestamptz NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX telegram_tokens_expires_idx ON telegram_tokens (expires_at);

-- +goose Down
DROP TABLE telegram_tokens;
DROP TABLE telegram_accounts;
