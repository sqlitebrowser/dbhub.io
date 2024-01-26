BEGIN;

UPDATE api_call_log SET api_operation = '/v1/' || api_operation;

COMMIT;
