DROP INDEX IF EXISTS idx_conversations_last_message_at;
DROP INDEX IF EXISTS idx_conversations_direct_key;
ALTER TABLE conversations DROP COLUMN IF EXISTS direct_key;
ALTER TABLE conversations DROP COLUMN IF EXISTS last_message_at;
