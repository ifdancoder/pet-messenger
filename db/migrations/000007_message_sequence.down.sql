CREATE INDEX idx_messages_conversation_created_id ON messages (conversation_id, created_at, id);
DROP INDEX IF EXISTS idx_messages_conversation_seq;
ALTER TABLE messages DROP COLUMN seq;
ALTER TABLE conversations DROP COLUMN next_seq;
