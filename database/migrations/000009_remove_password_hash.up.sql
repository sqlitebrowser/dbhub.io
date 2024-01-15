BEGIN;

ALTER TABLE users DROP COLUMN password_hash;

COMMIT;
