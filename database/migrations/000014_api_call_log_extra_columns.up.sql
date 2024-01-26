BEGIN;

ALTER TABLE api_call_log ADD COLUMN method text;
ALTER TABLE api_call_log ADD COLUMN status_code int;
ALTER TABLE api_call_log ADD COLUMN runtime bigint;
ALTER TABLE api_call_log ADD COLUMN request_size bigint;
ALTER TABLE api_call_log ADD COLUMN response_size bigint;

COMMIT;
