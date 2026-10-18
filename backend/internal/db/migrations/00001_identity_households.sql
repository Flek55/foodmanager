-- Users, authentication and households (multi-tenant core).
-- IDs use Postgres 18 native uuidv7(): time-ordered, index-friendly.

-- +goose Up
CREATE TABLE users (
    id                uuid PRIMARY KEY DEFAULT uuidv7(),
    display_name      text        NOT NULL,
    -- Nullable: a user who signed up via Telegram may have no email.
    email             text,
    email_verified_at timestamptz,
    -- Nullable: users who only use Google/Telegram have no password.
    password_hash     text,
    avatar_url        text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_key ON users (lower(email)) WHERE email IS NOT NULL;

-- External login providers linked to a user. One user can have several.
-- For telegram, subject is the Telegram user id, which is also the chat id
-- for private messages, so the bot can notify the user.
CREATE TABLE auth_identities (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider   text        NOT NULL CHECK (provider IN ('google', 'apple', 'telegram')),
    subject    text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, subject),
    UNIQUE (user_id, provider)
);

-- Server-side sessions. The cookie holds a random token; we store only its
-- SHA-256, so a leaked DB dump can't be used to hijack sessions.
CREATE TABLE sessions (
    token_hash   bytea PRIMARY KEY,
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    user_agent   text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE households (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    name       text        NOT NULL,
    -- Monotonic change counter for offline sync: every write to household
    -- data bumps it, and clients pull "everything since version N".
    version    bigint      NOT NULL DEFAULT 0,
    created_by uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
    household_id uuid        NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role         text        NOT NULL CHECK (role IN ('owner', 'member')),
    joined_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (household_id, user_id)
);
CREATE INDEX memberships_user_id_idx ON memberships (user_id);

-- Short human-friendly invite codes (e.g. "K7QM-2XPA").
CREATE TABLE invites (
    code         text PRIMARY KEY,
    household_id uuid        NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    created_by   uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    max_uses     int         NOT NULL DEFAULT 10 CHECK (max_uses > 0),
    uses         int         NOT NULL DEFAULT 0 CHECK (uses >= 0),
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX invites_household_id_idx ON invites (household_id);

-- +goose Down
DROP TABLE invites;
DROP TABLE memberships;
DROP TABLE households;
DROP TABLE sessions;
DROP TABLE auth_identities;
DROP TABLE users;
