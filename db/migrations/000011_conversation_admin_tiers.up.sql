-- One implicit transaction. CREATE TYPE of a brand-new enum and its immediate
-- use are fine in the same transaction (the "new value unusable in the same tx"
-- rule applies only to ALTER TYPE ... ADD VALUE on a pre-existing type).
-- The conversations DB is pre-launch; the ADD COLUMN / CREATE INDEX locks are
-- instant.

CREATE TYPE conversation_admin_tier AS ENUM
    ('creator_admin', 'superadmin', 'admin', 'content_admin');

ALTER TABLE conversation_members
    ADD COLUMN is_creator BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN admin_tier conversation_admin_tier;

-- Backfill from the legacy role values, then collapse them: role now means only
-- member / subscriber (it feeds CanPost). 'owner' / 'admin' stay defined on the
-- conversation_role enum but unused.
UPDATE conversation_members SET is_creator = true, admin_tier = 'creator_admin' WHERE role = 'owner';
UPDATE conversation_members SET admin_tier = 'admin' WHERE role = 'admin';
UPDATE conversation_members SET role = 'member' WHERE role IN ('owner', 'admin');

-- At most one creator per conversation.
CREATE UNIQUE INDEX idx_conversation_members_one_creator
    ON conversation_members (conversation_id) WHERE is_creator;

-- Only the creator row may hold creator_admin.
ALTER TABLE conversation_members
    ADD CONSTRAINT conversation_members_creator_admin_only
    CHECK (admin_tier IS DISTINCT FROM 'creator_admin' OR is_creator);
