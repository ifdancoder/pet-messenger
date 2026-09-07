CREATE TYPE message_type AS ENUM ('text', 'attachment', 'system', 'call_event');

CREATE TABLE messages (
    id              UUID PRIMARY KEY,
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id       UUID REFERENCES users(id) ON DELETE SET NULL,
    type            message_type NOT NULL,
    content         TEXT NOT NULL DEFAULT '',
    reply_to_id     UUID REFERENCES messages(id) ON DELETE SET NULL,
    edited_at       TIMESTAMPTZ,
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_messages_conversation_created_id ON messages (conversation_id, created_at, id);
