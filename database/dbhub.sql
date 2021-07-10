--
-- PostgreSQL database dump
--

-- Dumped from database version 12.6
-- Dumped by pg_dump version 12.6

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: jsquery; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS jsquery WITH SCHEMA public;


--
-- Name: EXTENSION jsquery; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION jsquery IS 'data type for jsonb inspection';


--
-- Name: permissions; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.permissions AS ENUM (
    'r',
    'rw'
);


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: api_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_keys (
    key_id bigint NOT NULL,
    user_id bigint,
    key text NOT NULL,
    date_created timestamp with time zone
);


--
-- Name: api_keys_key_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.api_keys_key_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: api_keys_key_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.api_keys_key_id_seq OWNED BY public.api_keys.key_id;


--
-- Name: api_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_permissions (
    key_id bigint,
    user_id bigint,
    db_id bigint,
    permissions jsonb
);


--
-- Name: database_downloads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.database_downloads (
    dl_id bigint NOT NULL,
    db_id bigint NOT NULL,
    user_id bigint,
    ip_addr text NOT NULL,
    server_sw text NOT NULL,
    user_agent text NOT NULL,
    download_date timestamp with time zone NOT NULL,
    db_sha256 text NOT NULL
);


--
-- Name: database_downloads_dl_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.database_downloads_dl_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: database_downloads_dl_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.database_downloads_dl_id_seq OWNED BY public.database_downloads.dl_id;


--
-- Name: database_files; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.database_files (
    db_sha256 text NOT NULL,
    minio_server text NOT NULL,
    minio_folder text NOT NULL,
    minio_id text NOT NULL
);


--
-- Name: database_licences; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.database_licences (
    lic_sha256 text NOT NULL,
    friendly_name text NOT NULL,
    user_id bigint NOT NULL,
    licence_url text,
    licence_text text NOT NULL,
    display_order integer,
    lic_id integer NOT NULL,
    full_name text,
    file_format text
);


--
-- Name: database_licences_lic_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.database_licences_lic_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: database_licences_lic_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.database_licences_lic_id_seq OWNED BY public.database_licences.lic_id;


--
-- Name: database_shares; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.database_shares (
    db_id bigint NOT NULL,
    user_id bigint NOT NULL,
    date_last_change timestamp with time zone DEFAULT now() NOT NULL,
    access public.permissions NOT NULL
);


--
-- Name: database_stars; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.database_stars (
    db_id bigint NOT NULL,
    user_id bigint NOT NULL,
    date_starred timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: database_uploads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.database_uploads (
    up_id bigint NOT NULL,
    db_id bigint NOT NULL,
    user_id bigint,
    ip_addr text NOT NULL,
    server_sw text NOT NULL,
    user_agent text NOT NULL,
    upload_date timestamp with time zone NOT NULL,
    db_sha256 text NOT NULL
);


--
-- Name: database_uploads_up_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.database_uploads_up_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: database_uploads_up_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.database_uploads_up_id_seq OWNED BY public.database_uploads.up_id;


--
-- Name: db4s_connects; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.db4s_connects (
    connect_id bigint NOT NULL,
    user_id integer,
    ip_addr text,
    user_agent text,
    connect_date timestamp with time zone
);


--
-- Name: db4s_connects_connect_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.db4s_connects_connect_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: db4s_connects_connect_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.db4s_connects_connect_id_seq OWNED BY public.db4s_connects.connect_id;


--
-- Name: discussion_comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.discussion_comments (
    com_id bigint NOT NULL,
    disc_id bigint NOT NULL,
    commenter bigint NOT NULL,
    date_created timestamp with time zone DEFAULT now() NOT NULL,
    body text NOT NULL,
    db_id bigint,
    entry_type text DEFAULT 'txt'::text NOT NULL
);


--
-- Name: discussion_comments_com_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.discussion_comments_com_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: discussion_comments_com_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.discussion_comments_com_id_seq OWNED BY public.discussion_comments.com_id;


--
-- Name: discussions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.discussions (
    internal_id bigint NOT NULL,
    db_id bigint NOT NULL,
    creator bigint NOT NULL,
    date_created timestamp with time zone DEFAULT now() NOT NULL,
    title text NOT NULL,
    description text NOT NULL,
    open boolean DEFAULT true NOT NULL,
    disc_id integer DEFAULT 1 NOT NULL,
    last_modified timestamp with time zone DEFAULT now() NOT NULL,
    comment_count integer DEFAULT 0 NOT NULL,
    discussion_type integer DEFAULT 0 NOT NULL,
    mr_source_db_id bigint,
    mr_source_db_branch text,
    mr_destination_branch text,
    mr_state integer DEFAULT 0 NOT NULL,
    mr_commits jsonb
);


--
-- Name: COLUMN discussions.mr_source_db_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.discussions.mr_source_db_id IS 'Only used by Merge Requests, not standard discussions';


--
-- Name: discussions_disc_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.discussions_disc_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: discussions_disc_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.discussions_disc_id_seq OWNED BY public.discussions.internal_id;


--
-- Name: email_queue; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.email_queue (
    email_id bigint NOT NULL,
    queued_timestamp timestamp with time zone DEFAULT now() NOT NULL,
    mail_to text NOT NULL,
    body text NOT NULL,
    sent boolean DEFAULT false NOT NULL,
    sent_timestamp timestamp with time zone,
    subject text NOT NULL
);


--
-- Name: email_queue_email_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.email_queue_email_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: email_queue_email_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.email_queue_email_id_seq OWNED BY public.email_queue.email_id;


--
-- Name: events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.events (
    event_id bigint NOT NULL,
    db_id bigint,
    event_type integer NOT NULL,
    event_data jsonb NOT NULL,
    event_timestamp timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: events_event_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.events_event_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: events_event_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.events_event_id_seq OWNED BY public.events.event_id;


--
-- Name: sqlite_databases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sqlite_databases (
    user_id bigint NOT NULL,
    db_id bigint NOT NULL,
    folder text NOT NULL,
    db_name text NOT NULL,
    public boolean DEFAULT false NOT NULL,
    date_created timestamp with time zone DEFAULT now() NOT NULL,
    last_modified timestamp with time zone DEFAULT now() NOT NULL,
    watchers bigint DEFAULT 0 NOT NULL,
    stars bigint DEFAULT 0 NOT NULL,
    forks bigint DEFAULT 0 NOT NULL,
    discussions bigint DEFAULT 0 NOT NULL,
    merge_requests bigint DEFAULT 0 NOT NULL,
    branches bigint DEFAULT 1 NOT NULL,
    contributors bigint DEFAULT 1 NOT NULL,
    one_line_description text,
    full_description text,
    root_database bigint,
    forked_from bigint,
    default_table text,
    source_url text,
    commit_list jsonb,
    branch_heads jsonb,
    tag_list jsonb,
    default_branch text,
    is_deleted boolean DEFAULT false NOT NULL,
    tags integer DEFAULT 0 NOT NULL,
    release_list jsonb,
    release_count integer DEFAULT 0 NOT NULL,
    download_count bigint DEFAULT 0,
    page_views bigint DEFAULT 0
);


--
-- Name: sqlite_databases_db_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.sqlite_databases_db_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sqlite_databases_db_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.sqlite_databases_db_id_seq OWNED BY public.sqlite_databases.db_id;


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    user_id bigint NOT NULL,
    user_name text NOT NULL,
    auth0_id text NOT NULL,
    email text,
    date_joined timestamp with time zone DEFAULT now() NOT NULL,
    client_cert bytea NOT NULL,
    password_hash text NOT NULL,
    pref_max_rows integer DEFAULT 10 NOT NULL,
    watchers bigint DEFAULT 0 NOT NULL,
    default_licence integer,
    display_name text,
    avatar_url text,
    status_updates jsonb
);


--
-- Name: users_user_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.users_user_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: users_user_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.users_user_id_seq OWNED BY public.users.user_id;


--
-- Name: vis_params; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vis_params (
    db_id bigint,
    user_id bigint,
    name text NOT NULL,
    date_created timestamp with time zone DEFAULT now(),
    parameters jsonb
);


--
-- Name: vis_query_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vis_query_runs (
    query_run_id bigint NOT NULL,
    db_id bigint,
    user_id bigint,
    ip_addr text,
    user_agent text,
    query_string text,
    query_date timestamp with time zone DEFAULT now(),
    memory_used bigint,
    memory_high_water bigint,
    source text
);


--
-- Name: vis_query_runs_query_run_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.vis_query_runs_query_run_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: vis_query_runs_query_run_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.vis_query_runs_query_run_id_seq OWNED BY public.vis_query_runs.query_run_id;


--
-- Name: vis_result_cache; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vis_result_cache (
    db_id bigint NOT NULL,
    user_id bigint NOT NULL,
    commit_id text NOT NULL,
    hash text NOT NULL,
    results jsonb
);


--
-- Name: watchers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.watchers (
    db_id bigint NOT NULL,
    user_id bigint NOT NULL,
    date_watched timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: api_keys key_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys ALTER COLUMN key_id SET DEFAULT nextval('public.api_keys_key_id_seq'::regclass);


--
-- Name: database_downloads dl_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_downloads ALTER COLUMN dl_id SET DEFAULT nextval('public.database_downloads_dl_id_seq'::regclass);


--
-- Name: database_licences lic_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_licences ALTER COLUMN lic_id SET DEFAULT nextval('public.database_licences_lic_id_seq'::regclass);


--
-- Name: database_uploads up_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_uploads ALTER COLUMN up_id SET DEFAULT nextval('public.database_uploads_up_id_seq'::regclass);


--
-- Name: db4s_connects connect_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.db4s_connects ALTER COLUMN connect_id SET DEFAULT nextval('public.db4s_connects_connect_id_seq'::regclass);


--
-- Name: discussion_comments com_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discussion_comments ALTER COLUMN com_id SET DEFAULT nextval('public.discussion_comments_com_id_seq'::regclass);


--
-- Name: discussions internal_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discussions ALTER COLUMN internal_id SET DEFAULT nextval('public.discussions_disc_id_seq'::regclass);


--
-- Name: email_queue email_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_queue ALTER COLUMN email_id SET DEFAULT nextval('public.email_queue_email_id_seq'::regclass);


--
-- Name: events event_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events ALTER COLUMN event_id SET DEFAULT nextval('public.events_event_id_seq'::regclass);


--
-- Name: sqlite_databases db_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sqlite_databases ALTER COLUMN db_id SET DEFAULT nextval('public.sqlite_databases_db_id_seq'::regclass);


--
-- Name: users user_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users ALTER COLUMN user_id SET DEFAULT nextval('public.users_user_id_seq'::regclass);


--
-- Name: vis_query_runs query_run_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vis_query_runs ALTER COLUMN query_run_id SET DEFAULT nextval('public.vis_query_runs_query_run_id_seq'::regclass);


--
-- Name: api_keys api_keys_key_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_key_id UNIQUE (key_id);


--
-- Name: api_keys api_keys_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_pk PRIMARY KEY (key);


--
-- Name: database_downloads database_downloads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_downloads
    ADD CONSTRAINT database_downloads_pkey PRIMARY KEY (dl_id);


--
-- Name: database_files database_files_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_files
    ADD CONSTRAINT database_files_pkey PRIMARY KEY (db_sha256);


--
-- Name: database_licences database_licences_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_licences
    ADD CONSTRAINT database_licences_pkey PRIMARY KEY (user_id, friendly_name);


--
-- Name: database_shares database_shares_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_shares
    ADD CONSTRAINT database_shares_pkey PRIMARY KEY (db_id, user_id);


--
-- Name: database_stars database_stars_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_stars
    ADD CONSTRAINT database_stars_pkey PRIMARY KEY (db_id, user_id);


--
-- Name: database_uploads database_uploads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_uploads
    ADD CONSTRAINT database_uploads_pkey PRIMARY KEY (up_id);


--
-- Name: discussion_comments discussion_comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discussion_comments
    ADD CONSTRAINT discussion_comments_pkey PRIMARY KEY (com_id);


--
-- Name: discussions discussions_db_id_disc_id_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discussions
    ADD CONSTRAINT discussions_db_id_disc_id_unique UNIQUE (db_id, disc_id);


--
-- Name: discussions discussions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discussions
    ADD CONSTRAINT discussions_pkey PRIMARY KEY (internal_id);


--
-- Name: email_queue email_queue_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_queue
    ADD CONSTRAINT email_queue_pkey PRIMARY KEY (email_id);


--
-- Name: events events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events
    ADD CONSTRAINT events_pkey PRIMARY KEY (event_id);


--
-- Name: sqlite_databases sqlite_databases_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sqlite_databases
    ADD CONSTRAINT sqlite_databases_pkey PRIMARY KEY (db_id);


--
-- Name: sqlite_databases sqlite_databases_user_id_folder_db_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sqlite_databases
    ADD CONSTRAINT sqlite_databases_user_id_folder_db_name_key UNIQUE (user_id, folder, db_name);


--
-- Name: users users_auth0_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_auth0_id_key UNIQUE (auth0_id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (user_id);


--
-- Name: users users_user_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_user_name_key UNIQUE (user_name);


--
-- Name: vis_params vis_params_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vis_params
    ADD CONSTRAINT vis_params_pk UNIQUE (db_id, user_id, name);


--
-- Name: vis_query_runs vis_query_runs_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vis_query_runs
    ADD CONSTRAINT vis_query_runs_pk PRIMARY KEY (query_run_id);


--
-- Name: vis_result_cache vis_result_cache_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vis_result_cache
    ADD CONSTRAINT vis_result_cache_pk PRIMARY KEY (db_id, user_id, commit_id, hash);


--
-- Name: watchers watchers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.watchers
    ADD CONSTRAINT watchers_pkey PRIMARY KEY (db_id, user_id);


--
-- Name: api_keys_key_uindex; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX api_keys_key_uindex ON public.api_keys USING btree (key);


--
-- Name: database_licences_lic_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX database_licences_lic_id_idx ON public.database_licences USING btree (lic_id);


--
-- Name: database_licences_lic_sha256_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX database_licences_lic_sha256_idx ON public.database_licences USING btree (lic_sha256);


--
-- Name: database_licences_user_id_friendly_name_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX database_licences_user_id_friendly_name_idx ON public.database_licences USING btree (user_id, friendly_name);


--
-- Name: db4s_connects_connect_date_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX db4s_connects_connect_date_index ON public.db4s_connects USING btree (connect_date);


--
-- Name: db4s_connects_user_id_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX db4s_connects_user_id_index ON public.db4s_connects USING btree (user_id);


--
-- Name: discussions_discussion_type_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX discussions_discussion_type_idx ON public.discussions USING btree (discussion_type);


--
-- Name: events_event_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX events_event_id_idx ON public.events USING btree (event_id);


--
-- Name: fki_database_downloads_db_id_fkey; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fki_database_downloads_db_id_fkey ON public.database_downloads USING btree (db_id);


--
-- Name: fki_database_downloads_user_id_fkey; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fki_database_downloads_user_id_fkey ON public.database_downloads USING btree (user_id);


--
-- Name: fki_database_uploads_db_id_fkey; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fki_database_uploads_db_id_fkey ON public.database_uploads USING btree (db_id);


--
-- Name: fki_database_uploads_user_id_fkey; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fki_database_uploads_user_id_fkey ON public.database_uploads USING btree (user_id);


--
-- Name: fki_discussion_comments_db_id_fkey; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fki_discussion_comments_db_id_fkey ON public.discussion_comments USING btree (db_id);


--
-- Name: fki_discussions_source_db_id_fkey; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fki_discussions_source_db_id_fkey ON public.discussions USING btree (mr_source_db_id);


--
-- Name: users_lower_user_name_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX users_lower_user_name_idx ON public.users USING btree (lower(user_name));


--
-- Name: users_user_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX users_user_id_idx ON public.users USING btree (user_id);


--
-- Name: users_user_name_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX users_user_name_idx ON public.users USING btree (user_name);


--
-- Name: vis_query_runs_query_date_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX vis_query_runs_query_date_index ON public.vis_query_runs USING btree (query_date);


--
-- Name: watchers_db_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX watchers_db_id_idx ON public.watchers USING btree (db_id);


--
-- Name: api_keys api_keys_users_user_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_users_user_id_fk FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON UPDATE CASCADE ON DELETE SET NULL;


--
-- Name: api_permissions api_permissions_api_keys_key_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_permissions
    ADD CONSTRAINT api_permissions_api_keys_key_id_fk FOREIGN KEY (key_id) REFERENCES public.api_keys(key_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: api_permissions api_permissions_sqlite_databases_db_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_permissions
    ADD CONSTRAINT api_permissions_sqlite_databases_db_id_fk FOREIGN KEY (db_id) REFERENCES public.sqlite_databases(db_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: api_permissions api_permissions_users_user_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_permissions
    ADD CONSTRAINT api_permissions_users_user_id_fk FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_downloads database_downloads_db_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_downloads
    ADD CONSTRAINT database_downloads_db_id_fkey FOREIGN KEY (db_id) REFERENCES public.sqlite_databases(db_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_downloads database_downloads_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_downloads
    ADD CONSTRAINT database_downloads_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_licences database_licences_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_licences
    ADD CONSTRAINT database_licences_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_shares database_shares_db_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_shares
    ADD CONSTRAINT database_shares_db_id_fkey FOREIGN KEY (db_id) REFERENCES public.sqlite_databases(db_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_shares database_shares_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_shares
    ADD CONSTRAINT database_shares_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_stars database_stars_db_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_stars
    ADD CONSTRAINT database_stars_db_id_fkey FOREIGN KEY (db_id) REFERENCES public.sqlite_databases(db_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_stars database_stars_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_stars
    ADD CONSTRAINT database_stars_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_uploads database_uploads_db_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_uploads
    ADD CONSTRAINT database_uploads_db_id_fkey FOREIGN KEY (db_id) REFERENCES public.sqlite_databases(db_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_uploads database_uploads_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.database_uploads
    ADD CONSTRAINT database_uploads_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: discussion_comments discussion_comments_commenter_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discussion_comments
    ADD CONSTRAINT discussion_comments_commenter_fkey FOREIGN KEY (commenter) REFERENCES public.users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: discussion_comments discussion_comments_db_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discussion_comments
    ADD CONSTRAINT discussion_comments_db_id_fkey FOREIGN KEY (db_id) REFERENCES public.sqlite_databases(db_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: discussion_comments discussion_comments_disc_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discussion_comments
    ADD CONSTRAINT discussion_comments_disc_id_fkey FOREIGN KEY (disc_id) REFERENCES public.discussions(internal_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: discussions discussions_db_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discussions
    ADD CONSTRAINT discussions_db_id_fkey FOREIGN KEY (db_id) REFERENCES public.sqlite_databases(db_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: discussions discussions_mr_source_db_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discussions
    ADD CONSTRAINT discussions_mr_source_db_id_fkey FOREIGN KEY (mr_source_db_id) REFERENCES public.sqlite_databases(db_id) ON UPDATE SET NULL ON DELETE SET NULL;


--
-- Name: discussions discussions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discussions
    ADD CONSTRAINT discussions_user_id_fkey FOREIGN KEY (creator) REFERENCES public.users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: events events_db_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events
    ADD CONSTRAINT events_db_id_fkey FOREIGN KEY (db_id) REFERENCES public.sqlite_databases(db_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: sqlite_databases sqlite_databases_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sqlite_databases
    ADD CONSTRAINT sqlite_databases_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: vis_params vis_params_users_user_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vis_params
    ADD CONSTRAINT vis_params_users_user_id_fk FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: vis_result_cache vis_result_cache_users_user_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vis_result_cache
    ADD CONSTRAINT vis_result_cache_users_user_id_fk FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: watchers watchers_db_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.watchers
    ADD CONSTRAINT watchers_db_id_fkey FOREIGN KEY (db_id) REFERENCES public.sqlite_databases(db_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: watchers watchers_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.watchers
    ADD CONSTRAINT watchers_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

