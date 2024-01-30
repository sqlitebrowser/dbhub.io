BEGIN;

ALTER TABLE usage_limits ADD COLUMN max_upload_size bigint;

UPDATE usage_limits SET max_upload_size = NULL WHERE id = 1;
UPDATE usage_limits SET max_upload_size = 512 * 1024 * 1024 WHERE id = 2;
UPDATE usage_limits SET max_upload_size = 0 WHERE id = 3;

COMMIT;
