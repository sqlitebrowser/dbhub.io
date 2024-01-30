BEGIN;

ALTER TABLE usage_limits DROP COLUMN max_upload_size;

COMMIT;
