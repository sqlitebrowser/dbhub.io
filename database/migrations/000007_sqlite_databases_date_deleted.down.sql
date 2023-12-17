BEGIN;
ALTER TABLE sqlite_databases DROP COLUMN IF EXISTS date_deleted;
COMMIT;