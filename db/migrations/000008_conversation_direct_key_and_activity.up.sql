-- Runs as one implicit transaction. The backfill UPDATEs and the CREATE INDEX
-- statements briefly take an ACCESS EXCLUSIVE lock on `conversations`; that is
-- deliberate and safe for the current pre-launch, empty database. Against a
-- populated table this would need splitting (add nullable column -> batched
-- backfill -> CREATE INDEX CONCURRENTLY -> SET NOT NULL via a NOT VALID check
-- then VALIDATE), none of which can share one transaction.

ALTER TABLE conversations ADD COLUMN last_message_at TIMESTAMPTZ;

UPDATE conversations c
SET last_message_at = COALESCE(
    (SELECT max(m.created_at) FROM messages m WHERE m.conversation_id = c.id),
    c.created_at
);

ALTER TABLE conversations ALTER COLUMN last_message_at SET NOT NULL;
ALTER TABLE conversations ALTER COLUMN last_message_at SET DEFAULT now();

ALTER TABLE conversations ADD COLUMN direct_key TEXT;

-- Backfill direct_key for any existing direct conversation: the smaller member
-- uuid, a colon, the larger. Direct conversations always have exactly two
-- members (conversation.NewDirect), so the pair is well-defined.
UPDATE conversations c
SET direct_key = sub.k
FROM (
    SELECT a.conversation_id AS cid,
           least(a.user_id, b.user_id)::text || ':' || greatest(a.user_id, b.user_id)::text AS k
    FROM conversation_members a
    JOIN conversation_members b
      ON b.conversation_id = a.conversation_id AND b.user_id > a.user_id
) sub
WHERE c.id = sub.cid AND c.kind = 'direct';

-- Order-independent uniqueness for direct conversations; partial so group chats
-- and channels (direct_key IS NULL) are unaffected.
CREATE UNIQUE INDEX idx_conversations_direct_key
    ON conversations (direct_key)
    WHERE kind = 'direct';

-- The partial unique index treats NULLs as distinct, so a `direct` row with a
-- NULL direct_key would slip past the "one DM per pair" invariant. Forbid that
-- combination outright.
ALTER TABLE conversations
    ADD CONSTRAINT conversations_direct_key_present
    CHECK (kind <> 'direct' OR direct_key IS NOT NULL);

-- Supports the conversation-list keyset scan: ORDER BY last_message_at DESC, id DESC.
CREATE INDEX idx_conversations_last_message_at
    ON conversations (last_message_at DESC, id DESC);
