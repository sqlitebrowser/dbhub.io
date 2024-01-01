BEGIN;

UPDATE api_keys SET key = encode(sha256(key::bytea), 'hex');

COMMIT;
