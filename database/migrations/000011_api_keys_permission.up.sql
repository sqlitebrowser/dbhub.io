BEGIN;

ALTER TABLE api_keys ADD COLUMN permissions public.permissions NOT NULL DEFAULT 'rw';

COMMIT;
