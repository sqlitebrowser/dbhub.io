BEGIN;
DROP TRIGGER track_applied_migrations ON schema_migrations;
DROP FUNCTION track_applied_migration();
DROP TABLE schema_migrations_history;
COMMIT;