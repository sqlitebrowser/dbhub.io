BEGIN;

ALTER TABLE users ADD COLUMN client_cert bytea NOT NULL DEFAULT ''::bytea;

COMMIT;
