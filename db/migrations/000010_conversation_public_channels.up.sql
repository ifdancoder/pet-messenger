-- One implicit transaction. ADD COLUMN with a constant DEFAULT and CREATE INDEX
-- on the current pre-launch (empty) conversations table are instant.

ALTER TABLE conversations ADD COLUMN public BOOLEAN NOT NULL DEFAULT false;

-- Backs the channel-discovery keyset scan (GET /conversations/channels):
-- ORDER BY last_message_at DESC, id DESC over public channels only.
CREATE INDEX idx_conversations_public_channels
    ON conversations (last_message_at DESC, id DESC)
    WHERE public AND kind IN ('channel_group', 'channel_broadcast');
