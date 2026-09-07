CREATE TYPE conversation_kind AS ENUM ('direct', 'group_chat', 'channel_group', 'channel_broadcast');
CREATE TYPE conversation_role AS ENUM ('owner', 'admin', 'member', 'subscriber');

CREATE TABLE conversations (
    id         UUID PRIMARY KEY,
    kind       conversation_kind NOT NULL,
    title      TEXT NOT NULL DEFAULT '',
    avatar_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL
);
