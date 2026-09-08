-- One implicit transaction. On the current pre-launch database `messages` is
-- empty, so ADD COLUMN and CREATE INDEX are instant. The index is partial on
-- `idempotency_key IS NOT NULL`, so keyless messages (the common case) never
-- touch it.

ALTER TABLE messages ADD COLUMN idempotency_key TEXT;

ALTER TABLE messages
    ADD CONSTRAINT messages_idempotency_key_len
    CHECK (char_length(idempotency_key) <= 255);

-- Per-conversation send idempotency: a client that retries a send with the same
-- key gets the original row back instead of creating a duplicate.
CREATE UNIQUE INDEX idx_messages_conversation_idempotency_key
    ON messages (conversation_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
