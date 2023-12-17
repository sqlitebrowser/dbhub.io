BEGIN;
ALTER TABLE analysis_space_used DROP COLUMN IF EXISTS standard_databases_qty;
ALTER TABLE analysis_space_used DROP COLUMN IF EXISTS live_databases_qty;
COMMIT;