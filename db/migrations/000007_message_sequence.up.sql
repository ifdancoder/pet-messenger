ALTER TABLE conversations ADD COLUMN next_seq BIGINT NOT NULL DEFAULT 0;

ALTER TABLE messages ADD COLUMN seq BIGINT;

WITH numbered AS (
    SELECT id,
           row_number() OVER (PARTITION BY conversation_id ORDER BY created_at, id) AS rn
    FROM messages
)
UPDATE messages m
SET seq = n.rn
FROM numbered n
WHERE m.id = n.id;

ALTER TABLE messages ALTER COLUMN seq SET NOT NULL;

UPDATE conversations c
SET next_seq = COALESCE((SELECT max(m.seq) FROM messages m WHERE m.conversation_id = c.id), 0);

CREATE UNIQUE INDEX idx_messages_conversation_seq ON messages (conversation_id, seq);
DROP INDEX idx_messages_conversation_created_id;
