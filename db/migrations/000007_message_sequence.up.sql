-- This migration runs as a single implicit transaction. The backfill UPDATE and
-- the CREATE UNIQUE INDEX below both take an ACCESS EXCLUSIVE lock on `messages`
-- for the duration of that transaction. That is deliberate and safe for the
-- current pre-launch, empty database. Running it against a populated `messages`
-- table would instead need splitting into: add the column nullable -> backfill
-- in batches -> CREATE UNIQUE INDEX CONCURRENTLY -> ADD CONSTRAINT ... NOT VALID
-- followed by VALIDATE CONSTRAINT, none of which can share one transaction.

ALTER TABLE conversations ADD COLUMN last_seq BIGINT NOT NULL DEFAULT 0;

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
ALTER TABLE messages ADD CONSTRAINT messages_seq_positive CHECK (seq > 0);

UPDATE conversations c
SET last_seq = s.max_seq
FROM (SELECT conversation_id, max(seq) AS max_seq FROM messages GROUP BY conversation_id) s
WHERE s.conversation_id = c.id;

CREATE UNIQUE INDEX idx_messages_conversation_seq ON messages (conversation_id, seq);
DROP INDEX IF EXISTS idx_messages_conversation_created_id;
