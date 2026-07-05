CREATE TABLE users (
    id             UUID PRIMARY KEY,
    email          TEXT NOT NULL UNIQUE,
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    password_hash  TEXT NOT NULL,
    name           TEXT NOT NULL DEFAULT '',
    avatar_url     TEXT NOT NULL DEFAULT '',
    locale         TEXT NOT NULL DEFAULT 'en',
    timezone       TEXT NOT NULL DEFAULT 'UTC',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
