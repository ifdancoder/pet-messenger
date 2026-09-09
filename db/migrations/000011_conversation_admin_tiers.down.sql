-- Best-effort reconstruction of the legacy role values before the columns go.
UPDATE conversation_members SET role = 'owner' WHERE is_creator;
UPDATE conversation_members SET role = 'admin' WHERE admin_tier = 'admin' AND NOT is_creator;

DROP INDEX IF EXISTS idx_conversation_members_one_creator;

ALTER TABLE conversation_members
    DROP CONSTRAINT IF EXISTS conversation_members_creator_admin_only,
    DROP COLUMN IF EXISTS admin_tier,
    DROP COLUMN IF EXISTS is_creator;

DROP TYPE IF EXISTS conversation_admin_tier;
