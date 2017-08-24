--
-- PostgreSQL database dump
--

-- Dumped from database version 9.6.4
-- Dumped by pg_dump version 9.6.4

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET check_function_bodies = false;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: plpgsql; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS plpgsql WITH SCHEMA pg_catalog;


--
-- Name: EXTENSION plpgsql; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION plpgsql IS 'PL/pgSQL procedural language';


SET search_path = public, pg_catalog;

SET default_tablespace = '';

SET default_with_oids = false;

--
-- Name: database_downloads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE database_downloads (
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

CREATE SEQUENCE database_downloads_dl_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: database_downloads_dl_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE database_downloads_dl_id_seq OWNED BY database_downloads.dl_id;


--
-- Name: database_files; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE database_files (
    db_sha256 text NOT NULL,
    minio_server text NOT NULL,
    minio_folder text NOT NULL,
    minio_id text NOT NULL
);


--
-- Name: database_licences; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE database_licences (
    lic_sha256 text NOT NULL,
    friendly_name text NOT NULL,
    user_id bigint NOT NULL,
    licence_url text,
    licence_text text NOT NULL,
    display_order integer,
    lic_id integer NOT NULL,
    full_name text,
    file_format text DEFAULT 'text'::text NOT NULL
);


--
-- Name: database_licences_lic_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE database_licences_lic_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: database_licences_lic_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE database_licences_lic_id_seq OWNED BY database_licences.lic_id;


--
-- Name: database_stars; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE database_stars (
    db_id bigint NOT NULL,
    user_id bigint NOT NULL,
    date_starred timestamp with time zone DEFAULT timezone('utc'::text, now()) NOT NULL
);


--
-- Name: database_uploads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE database_uploads (
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

CREATE SEQUENCE database_uploads_up_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: database_uploads_up_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE database_uploads_up_id_seq OWNED BY database_uploads.up_id;


--
-- Name: discussion_comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE discussion_comments (
    com_id bigint NOT NULL,
    disc_id bigint NOT NULL,
    commenter bigint NOT NULL,
    date_created timestamp with time zone DEFAULT now() NOT NULL,
    body text NOT NULL,
    db_id bigint
);


--
-- Name: discussion_comments_com_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE discussion_comments_com_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: discussion_comments_com_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE discussion_comments_com_id_seq OWNED BY discussion_comments.com_id;


--
-- Name: discussions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE discussions (
    internal_id bigint NOT NULL,
    db_id bigint NOT NULL,
    creator bigint NOT NULL,
    date_created timestamp with time zone DEFAULT now() NOT NULL,
    title text NOT NULL,
    description text NOT NULL,
    open boolean DEFAULT true NOT NULL,
    disc_id integer DEFAULT 1 NOT NULL,
    last_modified timestamp with time zone DEFAULT now() NOT NULL,
    comment_count integer DEFAULT 0 NOT NULL
);


--
-- Name: discussions_disc_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE discussions_disc_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: discussions_disc_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE discussions_disc_id_seq OWNED BY discussions.internal_id;


--
-- Name: sqlite_databases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE sqlite_databases (
    user_id bigint NOT NULL,
    db_id bigint NOT NULL,
    folder text NOT NULL,
    db_name text NOT NULL,
    public boolean DEFAULT false NOT NULL,
    date_created timestamp with time zone DEFAULT timezone('utc'::text, now()) NOT NULL,
    last_modified timestamp with time zone DEFAULT timezone('utc'::text, now()) NOT NULL,
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

CREATE SEQUENCE sqlite_databases_db_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sqlite_databases_db_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE sqlite_databases_db_id_seq OWNED BY sqlite_databases.db_id;


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE users (
    user_id bigint NOT NULL,
    user_name text NOT NULL,
    auth0_id text NOT NULL,
    email text,
    date_joined timestamp with time zone DEFAULT timezone('utc'::text, now()) NOT NULL,
    client_cert bytea NOT NULL,
    password_hash text NOT NULL,
    pref_max_rows integer DEFAULT 10 NOT NULL,
    watchers bigint DEFAULT 0 NOT NULL,
    default_licence integer,
    display_name text
);


--
-- Name: users_user_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE users_user_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: users_user_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE users_user_id_seq OWNED BY users.user_id;


--
-- Name: database_downloads dl_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY database_downloads ALTER COLUMN dl_id SET DEFAULT nextval('database_downloads_dl_id_seq'::regclass);


--
-- Name: database_licences lic_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY database_licences ALTER COLUMN lic_id SET DEFAULT nextval('database_licences_lic_id_seq'::regclass);


--
-- Name: database_uploads up_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY database_uploads ALTER COLUMN up_id SET DEFAULT nextval('database_uploads_up_id_seq'::regclass);


--
-- Name: discussion_comments com_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY discussion_comments ALTER COLUMN com_id SET DEFAULT nextval('discussion_comments_com_id_seq'::regclass);


--
-- Name: discussions internal_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY discussions ALTER COLUMN internal_id SET DEFAULT nextval('discussions_disc_id_seq'::regclass);


--
-- Name: sqlite_databases db_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY sqlite_databases ALTER COLUMN db_id SET DEFAULT nextval('sqlite_databases_db_id_seq'::regclass);


--
-- Name: users user_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY users ALTER COLUMN user_id SET DEFAULT nextval('users_user_id_seq'::regclass);


--
-- Name: database_downloads database_downloads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY database_downloads
    ADD CONSTRAINT database_downloads_pkey PRIMARY KEY (dl_id);


--
-- Name: database_files database_files_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY database_files
    ADD CONSTRAINT database_files_pkey PRIMARY KEY (db_sha256);


--
-- Name: database_licences database_licences_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY database_licences
    ADD CONSTRAINT database_licences_pkey PRIMARY KEY (user_id, friendly_name);


--
-- Name: database_stars database_stars_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY database_stars
    ADD CONSTRAINT database_stars_pkey PRIMARY KEY (db_id, user_id);


--
-- Name: database_uploads database_uploads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY database_uploads
    ADD CONSTRAINT database_uploads_pkey PRIMARY KEY (up_id);


--
-- Name: discussion_comments discussion_comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY discussion_comments
    ADD CONSTRAINT discussion_comments_pkey PRIMARY KEY (com_id);


--
-- Name: discussions discussions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY discussions
    ADD CONSTRAINT discussions_pkey PRIMARY KEY (internal_id);


--
-- Name: sqlite_databases sqlite_databases_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY sqlite_databases
    ADD CONSTRAINT sqlite_databases_pkey PRIMARY KEY (db_id);


--
-- Name: sqlite_databases sqlite_databases_user_id_folder_db_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY sqlite_databases
    ADD CONSTRAINT sqlite_databases_user_id_folder_db_name_key UNIQUE (user_id, folder, db_name);


--
-- Name: users users_auth0_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY users
    ADD CONSTRAINT users_auth0_id_key UNIQUE (auth0_id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY users
    ADD CONSTRAINT users_pkey PRIMARY KEY (user_id);


--
-- Name: users users_user_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY users
    ADD CONSTRAINT users_user_name_key UNIQUE (user_name);


--
-- Name: database_licences_lic_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX database_licences_lic_id_idx ON database_licences USING btree (lic_id);


--
-- Name: database_licences_lic_sha256_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX database_licences_lic_sha256_idx ON database_licences USING btree (lic_sha256);


--
-- Name: database_licences_user_id_friendly_name_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX database_licences_user_id_friendly_name_idx ON database_licences USING btree (user_id, friendly_name);


--
-- Name: fki_database_downloads_db_id_fkey; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fki_database_downloads_db_id_fkey ON database_downloads USING btree (db_id);


--
-- Name: fki_database_downloads_user_id_fkey; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fki_database_downloads_user_id_fkey ON database_downloads USING btree (user_id);


--
-- Name: fki_database_uploads_db_id_fkey; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fki_database_uploads_db_id_fkey ON database_uploads USING btree (db_id);


--
-- Name: fki_database_uploads_user_id_fkey; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fki_database_uploads_user_id_fkey ON database_uploads USING btree (user_id);


--
-- Name: fki_discussion_comments_db_id_fkey; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fki_discussion_comments_db_id_fkey ON discussion_comments USING btree (db_id);


--
-- Name: users_user_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX users_user_id_idx ON users USING btree (user_id);


--
-- Name: users_user_name_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX users_user_name_idx ON users USING btree (user_name);


--
-- Name: database_downloads database_downloads_db_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY database_downloads
    ADD CONSTRAINT database_downloads_db_id_fkey FOREIGN KEY (db_id) REFERENCES sqlite_databases(db_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_downloads database_downloads_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY database_downloads
    ADD CONSTRAINT database_downloads_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_licences database_licences_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY database_licences
    ADD CONSTRAINT database_licences_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_stars database_stars_db_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY database_stars
    ADD CONSTRAINT database_stars_db_id_fkey FOREIGN KEY (db_id) REFERENCES sqlite_databases(db_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_stars database_stars_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY database_stars
    ADD CONSTRAINT database_stars_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_uploads database_uploads_db_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY database_uploads
    ADD CONSTRAINT database_uploads_db_id_fkey FOREIGN KEY (db_id) REFERENCES sqlite_databases(db_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_uploads database_uploads_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY database_uploads
    ADD CONSTRAINT database_uploads_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: discussion_comments discussion_comments_commenter_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY discussion_comments
    ADD CONSTRAINT discussion_comments_commenter_fkey FOREIGN KEY (commenter) REFERENCES users(user_id);


--
-- Name: discussion_comments discussion_comments_db_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY discussion_comments
    ADD CONSTRAINT discussion_comments_db_id_fkey FOREIGN KEY (db_id) REFERENCES sqlite_databases(db_id);


--
-- Name: discussion_comments discussion_comments_disc_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY discussion_comments
    ADD CONSTRAINT discussion_comments_disc_id_fkey FOREIGN KEY (disc_id) REFERENCES discussions(internal_id);


--
-- Name: discussions discussions_db_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY discussions
    ADD CONSTRAINT discussions_db_id_fkey FOREIGN KEY (db_id) REFERENCES sqlite_databases(db_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: discussions discussions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY discussions
    ADD CONSTRAINT discussions_user_id_fkey FOREIGN KEY (creator) REFERENCES users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: sqlite_databases sqlite_databases_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY sqlite_databases
    ADD CONSTRAINT sqlite_databases_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(user_id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

