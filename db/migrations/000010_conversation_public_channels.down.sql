DROP INDEX IF EXISTS idx_conversations_public_channels;
ALTER TABLE conversations DROP COLUMN IF EXISTS public;
