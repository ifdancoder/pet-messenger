-- This down migration is lossy by design. The admin_tier ladder and the
-- is_creator badge are dropped, not reconstructed into the legacy owner/admin
-- role values: a re-up would otherwise silently demote superadmin/content_admin
-- to no tier and silently promote a stepped-down creator back to creator_admin.
-- The database is pre-launch; a rollback that needs the old roles must restore
-- from a backup.

DROP INDEX IF EXISTS idx_conversation_members_one_creator;

ALTER TABLE conversation_members
    DROP CONSTRAINT IF EXISTS conversation_members_creator_admin_only,
    DROP COLUMN IF EXISTS admin_tier,
    DROP COLUMN IF EXISTS is_creator;

DROP TYPE IF EXISTS conversation_admin_tier;
