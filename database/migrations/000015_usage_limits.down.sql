BEGIN;

ALTER TABLE users DROP COLUMN usage_limit_id;

DROP TABLE IF EXISTS usage_limits;

DROP INDEX IF EXISTS api_call_log_caller_id_index;
DROP INDEX IF EXISTS api_call_log_api_call_date_index;

COMMIT;
