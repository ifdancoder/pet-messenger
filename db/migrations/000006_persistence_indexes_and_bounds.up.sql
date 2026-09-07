CREATE INDEX idx_messages_sender_id ON messages (sender_id);
CREATE INDEX idx_messages_reply_to_id ON messages (reply_to_id);
CREATE INDEX idx_conversation_members_last_read_message_id ON conversation_members (last_read_message_id);

ALTER TABLE conversations ADD CONSTRAINT conversations_title_len CHECK (char_length(title) <= 256);
ALTER TABLE messages ADD CONSTRAINT messages_content_len CHECK (char_length(content) <= 4096);
