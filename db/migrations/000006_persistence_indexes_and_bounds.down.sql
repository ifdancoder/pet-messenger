ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_content_len;
ALTER TABLE conversations DROP CONSTRAINT IF EXISTS conversations_title_len;
DROP INDEX IF EXISTS idx_conversation_members_last_read_message_id;
DROP INDEX IF EXISTS idx_messages_reply_to_id;
DROP INDEX IF EXISTS idx_messages_sender_id;
