DROP INDEX IF EXISTS idx_messages_conversation_idempotency_key;
ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_idempotency_key_len;
ALTER TABLE messages DROP COLUMN IF EXISTS idempotency_key;
