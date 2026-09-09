-- One implicit transaction. Pre-launch DB; ADD/CREATE are instant. A
-- brand-new CREATE TYPE used in the same transaction is safe.

CREATE TYPE notification_kind AS ENUM ('message', 'member_removed', 'member_tier_changed');
CREATE TYPE push_platform AS ENUM ('web', 'fcm', 'apns');

CREATE TABLE notifications (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       notification_kind NOT NULL,
    payload    JSONB NOT NULL,
    dedup_key  TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    read_at    TIMESTAMPTZ
);
CREATE INDEX idx_notifications_user_created
    ON notifications (user_id, created_at DESC, id DESC);
CREATE UNIQUE INDEX idx_notifications_user_dedup
    ON notifications (user_id, dedup_key) WHERE dedup_key IS NOT NULL;
CREATE INDEX idx_notifications_user_unread
    ON notifications (user_id) WHERE read_at IS NULL;

CREATE TABLE push_tokens (
    id           UUID PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform     push_platform NOT NULL,
    token        TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX idx_push_tokens_user_token ON push_tokens (user_id, token);
