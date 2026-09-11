-- ALTER TYPE ... ADD VALUE is the only statement here and the value is not
-- used in this file, so the single implicit transaction is fine on PG 12+.
ALTER TYPE notification_kind ADD VALUE IF NOT EXISTS 'call_started';
