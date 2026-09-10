-- Postgres cannot drop a single enum value. The down migration is a no-op;
-- a full rollback recreates the type via 000013 down.
SELECT 1;
