BEGIN;

-- Remove the foreign key and the key_id column
ALTER TABLE api_call_log DROP CONSTRAINT api_call_log_api_keys_key_id_fk;
ALTER TABLE api_call_log DROP COLUMN key_id;

-- Change the primary key of the api_keys table back to key
ALTER TABLE api_keys DROP CONSTRAINT api_keys_pkey;
ALTER TABLE api_keys ADD CONSTRAINT api_keys_pk PRIMARY KEY (key);

COMMIT;
