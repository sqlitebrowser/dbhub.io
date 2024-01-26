BEGIN;

UPDATE api_call_log SET api_operation = regexp_replace(api_operation, '^/v1/', '');

COMMIT;
