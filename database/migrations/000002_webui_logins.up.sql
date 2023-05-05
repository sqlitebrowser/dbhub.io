BEGIN;

CREATE TABLE IF NOT EXISTS public.webui_logins
(
    id         bigserial,
    user_id    bigint
        constraint webui_logins_users_user_id_fk
            references public.users (user_id),
    login_date timestamptz default now() not null
);

CREATE INDEX IF NOT EXISTS webui_logins_user_id_index
    on public.webui_logins (user_id);

COMMIT;