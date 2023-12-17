BEGIN;
ALTER TABLE sqlite_databases ADD date_deleted timestamptz;
UPDATE sqlite_databases SET date_deleted = last_modified WHERE is_deleted = true;
COMMIT;