BEGIN;

CREATE TABLE schema_migrations_history (
    id SERIAL PRIMARY KEY NOT NULL,
    version BIGINT NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT NOW()
);

CREATE OR REPLACE FUNCTION track_applied_migration()
    RETURNS TRIGGER AS $$
DECLARE _current_version integer;
BEGIN
    SELECT COALESCE(MAX(version),0) FROM schema_migrations_history INTO _current_version;
    IF new.dirty = 'f' AND new.version > _current_version THEN
        INSERT INTO schema_migrations_history(version) VALUES (new.version);
    END IF;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- TRIGGER
CREATE TRIGGER track_applied_migrations AFTER INSERT ON schema_migrations FOR EACH ROW EXECUTE PROCEDURE track_applied_migration();

COMMIT;
