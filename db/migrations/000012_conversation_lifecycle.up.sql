-- One implicit transaction. On the current pre-launch database the tables are
-- effectively empty, so ADD COLUMN / CREATE INDEX / CREATE TYPE are instant.
-- A brand-new CREATE TYPE ... AS ENUM is usable in the same transaction (only
-- ALTER TYPE ... ADD VALUE on a pre-existing type has that restriction).

ALTER TABLE conversations ADD COLUMN archived_at TIMESTAMPTZ;

-- Backs the (joined_at, user_id) keyset for GET /conversations/:id/members.
CREATE INDEX idx_conversation_members_conv_joined
    ON conversation_members (conversation_id, joined_at, user_id);

CREATE TYPE message_system_event AS ENUM (
    'member_joined', 'member_left', 'member_removed', 'member_tier_changed',
    'conversation_archived', 'conversation_unarchived'
);

ALTER TABLE messages
    ADD COLUMN system_event message_system_event,
    ADD COLUMN system_target_id UUID REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE messages
    ADD CONSTRAINT messages_system_event_type
    CHECK (system_event IS NULL OR type = 'system');
