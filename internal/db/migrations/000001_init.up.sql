CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    telegram_id BIGINT NOT NULL UNIQUE,
    username TEXT NOT NULL DEFAULT '',
    full_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS meetings (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    creator_id BIGINT NOT NULL REFERENCES users(id),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (ends_at > starts_at)
);

CREATE TABLE IF NOT EXISTS meeting_participants (
    meeting_id BIGINT NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (meeting_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_meetings_starts_at ON meetings (starts_at);
CREATE INDEX IF NOT EXISTS idx_meetings_ends_at ON meetings (ends_at);
CREATE INDEX IF NOT EXISTS idx_meeting_participants_user_id ON meeting_participants (user_id);
