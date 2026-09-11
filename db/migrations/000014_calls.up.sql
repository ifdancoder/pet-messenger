-- One implicit transaction. Pre-launch DB. A brand-new CREATE TYPE used in
-- the same transaction is safe.

CREATE TYPE call_status AS ENUM ('active', 'ended');
CREATE TYPE call_participant_role AS ENUM ('publisher', 'viewer');

CREATE TABLE calls (
    id              UUID PRIMARY KEY,
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    initiator_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status          call_status NOT NULL,
    media_node_id   TEXT NOT NULL,
    started_at      TIMESTAMPTZ NOT NULL,
    ended_at        TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_calls_one_live_per_conversation
    ON calls (conversation_id) WHERE status = 'active';
CREATE INDEX idx_calls_conversation ON calls (conversation_id);

CREATE TABLE call_participants (
    id         UUID PRIMARY KEY,
    call_id    UUID NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       call_participant_role NOT NULL,
    joined_at  TIMESTAMPTZ NOT NULL,
    left_at    TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_call_participants_one_active
    ON call_participants (call_id, user_id) WHERE left_at IS NULL;
CREATE INDEX idx_call_participants_call ON call_participants (call_id);
