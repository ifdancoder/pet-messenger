CREATE TABLE conversation_inbox (
    user_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    conversation_id      uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    kind                 conversation_kind NOT NULL,
    title                text NOT NULL DEFAULT '',
    avatar_url           text NOT NULL DEFAULT '',
    public               boolean NOT NULL DEFAULT false,
    created_at           timestamptz NOT NULL,
    archived_at          timestamptz,
    direct_peer_user_id  uuid REFERENCES users(id) ON DELETE SET NULL,
    last_message_at      timestamptz NOT NULL,
    last_event_seq       bigint NOT NULL DEFAULT 0,
    updated_at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, conversation_id)
);

CREATE INDEX idx_conversation_inbox_user_activity
    ON conversation_inbox (user_id, last_message_at DESC, conversation_id DESC)
    WHERE archived_at IS NULL;
