BEGIN;

ALTER TABLE api_call_log DROP COLUMN method;
ALTER TABLE api_call_log DROP COLUMN status_code;
ALTER TABLE api_call_log DROP COLUMN runtime;
ALTER TABLE api_call_log DROP COLUMN request_size;
ALTER TABLE api_call_log DROP COLUMN response_size;

COMMIT;
