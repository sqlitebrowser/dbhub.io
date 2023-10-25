BEGIN;
ALTER TABLE analysis_space_used ADD standard_databases_qty INTEGER;
ALTER TABLE analysis_space_used ADD live_databases_qty INTEGER;
COMMIT;