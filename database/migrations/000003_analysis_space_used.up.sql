BEGIN;
CREATE TABLE IF NOT EXISTS public.analysis_space_used
(
    id                       BIGSERIAL,
    user_id                  BIGINT NOT NULL
        CONSTRAINT analysis_space_used_users_user_id_fk REFERENCES public.users,
    analysis_date            TIMESTAMPTZ DEFAULT now(),
    standard_databases_bytes BIGINT,
    live_databases_bytes     BIGINT
);
COMMIT;