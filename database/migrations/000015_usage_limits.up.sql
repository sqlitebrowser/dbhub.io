BEGIN;

-- Create new table for usage limits
CREATE TABLE IF NOT EXISTS usage_limits (
    id bigserial PRIMARY KEY,
    name text NOT NULL,
    description text,
    rate_limits jsonb
);

-- Insert default usage limits
INSERT INTO usage_limits (name, description, rate_limits) VALUES
	('default', 'Default limits for new users', '[{"limit": 10, "period": "s", "increase": 10}]'),
	('unlimited', 'No usage limits (intended for testing and developers)', NULL);

-- Assign usage limits to users
ALTER TABLE users ADD COLUMN usage_limits_id bigint DEFAULT 1 CONSTRAINT users_usage_limits_usage_limits_id_fk REFERENCES usage_limits(id);

-- Create indexes on api_call_log since we start querying that table now
CREATE INDEX IF NOT EXISTS api_call_log_caller_id_index ON api_call_log (caller_id);
CREATE INDEX IF NOT EXISTS api_call_log_api_call_date_index ON api_call_log (api_call_date);

COMMIT;
