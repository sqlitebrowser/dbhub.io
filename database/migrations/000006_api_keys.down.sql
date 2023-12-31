BEGIN;

ALTER TABLE api_keys DROP COLUMN comment;

ALTER TABLE api_keys DROP COLUMN expiry_date;

ALTER TABLE api_keys DROP COLUMN uuid;

DROP INDEX IF EXISTS api_keys_uuid_uindex;

COMMIT;
