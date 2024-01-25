BEGIN;

ALTER TABLE api_keys DROP COLUMN permissions;

COMMIT;
