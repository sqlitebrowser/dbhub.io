BEGIN;

-- Change the primary key of the api_keys table from key to key_id
ALTER TABLE api_keys DROP CONSTRAINT api_keys_pk;
ALTER TABLE api_keys ADD CONSTRAINT api_keys_pkey PRIMARY KEY (key_id);

-- Add a column key_id to the table api_call_log and make add a reference from it to the api_keys table
ALTER TABLE api_call_log ADD COLUMN key_id bigint;

ALTER TABLE api_call_log
    ADD CONSTRAINT api_call_log_api_keys_key_id_fk FOREIGN KEY (key_id) REFERENCES api_keys(key_id) ON UPDATE CASCADE ON DELETE SET NULL;

COMMIT;
