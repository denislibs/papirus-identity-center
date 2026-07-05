CREATE TABLE sessions (
    id               UUID PRIMARY KEY,
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    hydra_session_id TEXT NOT NULL DEFAULT '',
    device_name      TEXT NOT NULL DEFAULT '',
    user_agent       TEXT NOT NULL DEFAULT '',
    ip               TEXT NOT NULL DEFAULT '',
    location         TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at         TIMESTAMPTZ
);
CREATE INDEX idx_sessions_user ON sessions (user_id) WHERE ended_at IS NULL;
CREATE INDEX idx_sessions_hydra ON sessions (hydra_session_id);
