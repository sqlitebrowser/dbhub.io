BEGIN;
DROP TABLE IF EXISTS job_submissions;
DROP TABLE IF EXISTS job_responses;
DROP TRIGGER IF EXISTS job_submissions_trigger ON job_submissions;
DROP TRIGGER IF EXISTS job_responses_trigger ON job_responses;
DROP FUNCTION IF EXISTS job_submissions_notify();
DROP FUNCTION IF EXISTS job_responses_notify();
COMMIT;