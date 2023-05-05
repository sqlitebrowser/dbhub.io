BEGIN;
DROP TRIGGER IF EXISTS track_applied_migrations ON schema_migrations;
DROP FUNCTION IF EXISTS track_applied_migration();
DROP TABLE IF EXISTS schema_migrations_history;
COMMIT;