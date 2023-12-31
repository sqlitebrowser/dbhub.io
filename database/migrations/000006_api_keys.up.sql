BEGIN;

ALTER TABLE api_keys ADD COLUMN uuid uuid NOT NULL DEFAULT gen_random_uuid();

ALTER TABLE api_keys ADD COLUMN expiry_date TIMESTAMP;

ALTER TABLE api_keys ADD COLUMN comment VARCHAR(255);

CREATE UNIQUE INDEX api_keys_uuid_uindex ON api_keys (uuid);

COMMIT;
