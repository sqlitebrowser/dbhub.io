BEGIN;

-- job_submissions table
CREATE TABLE IF NOT EXISTS job_submissions (
    job_id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    submission_date TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    target_node TEXT NOT NULL,
    submitter_node TEXT NOT NULL,
    operation TEXT NOT NULL,
    details JSONB,
    state TEXT NOT NULL DEFAULT 'new'::TEXT,
    completed_date TIMESTAMP WITH TIME ZONE
);

-- job_responses table
CREATE TABLE IF NOT EXISTS job_responses
(
    response_id BIGINT GENERATED BY DEFAULT AS IDENTITY
        CONSTRAINT job_responses_pk
            PRIMARY KEY,
    job_id BIGINT NOT NULL
        CONSTRAINT job_responses_job_submissions_job_id_fk
            REFERENCES job_submissions,
    response_date TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    submitter_node TEXT NOT NULL,
    details JSONB NOT NULL,
    processed_date TIMESTAMP WITH TIME ZONE
);

-- notify function for the job_submissions table
CREATE OR REPLACE FUNCTION job_submissions_notify()
    RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('job_submissions_queue', '');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- notify function for the job_responses table
CREATE OR REPLACE FUNCTION job_responses_notify()
    RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('job_responses_queue', NEW.submitter_node);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- trigger function for the job_submissions table
CREATE OR REPLACE TRIGGER job_submissions_trigger
    AFTER INSERT ON job_submissions
EXECUTE FUNCTION job_submissions_notify();

-- trigger function for the job_responses table
CREATE OR REPLACE TRIGGER job_responses_trigger
    AFTER INSERT ON job_responses
    FOR EACH ROW EXECUTE FUNCTION job_responses_notify();

COMMIT;