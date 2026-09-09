ALTER TABLE messages
    DROP CONSTRAINT IF EXISTS messages_system_event_type,
    DROP COLUMN IF EXISTS system_target_id,
    DROP COLUMN IF EXISTS system_event;

DROP TYPE IF EXISTS message_system_event;

DROP INDEX IF EXISTS idx_conversation_members_conv_joined;

ALTER TABLE conversations DROP COLUMN IF EXISTS archived_at;
