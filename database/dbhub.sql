--
-- PostgreSQL database cluster dump
--

SET default_transaction_read_only = off;

SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;

--
-- Roles
--

CREATE ROLE dbhub;
ALTER ROLE dbhub WITH SUPERUSER INHERIT CREATEROLE CREATEDB LOGIN NOREPLICATION NOBYPASSRLS PASSWORD 'md509be10e4087f5617d49b9d1fe3184a84';


--
-- Database creation
--

CREATE DATABASE dbhub WITH TEMPLATE = template0 OWNER = dbhub;
REVOKE CONNECT,TEMPORARY ON DATABASE template1 FROM PUBLIC;
GRANT CONNECT ON DATABASE template1 TO PUBLIC;


\connect dbhub

SET default_transaction_read_only = off;

--
-- PostgreSQL database dump
--

-- Dumped from database version 9.6.0
-- Dumped by pg_dump version 9.6.0

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET check_function_bodies = false;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: plpgsql; Type: EXTENSION; Schema: -; Owner: 
--

CREATE EXTENSION IF NOT EXISTS plpgsql WITH SCHEMA pg_catalog;


--
-- Name: EXTENSION plpgsql; Type: COMMENT; Schema: -; Owner: 
--

COMMENT ON EXTENSION plpgsql IS 'PL/pgSQL procedural language';


SET search_path = public, pg_catalog;

SET default_tablespace = '';

SET default_with_oids = true;

--
-- Name: database_stars; Type: TABLE; Schema: public; Owner: dbhub
--

CREATE TABLE database_stars (
    db bigint,
    username text,
    date_starred timestamp with time zone DEFAULT timezone('utc'::text, now())
);


ALTER TABLE database_stars OWNER TO dbhub;

SET default_with_oids = false;

--
-- Name: database_versions; Type: TABLE; Schema: public; Owner: dbhub
--

CREATE TABLE database_versions (
    idnum bigint NOT NULL,
    db integer NOT NULL,
    size bigint NOT NULL,
    version integer NOT NULL,
    sha256 text NOT NULL,
    minioid text NOT NULL,
    date_created timestamp with time zone DEFAULT timezone('utc'::text, now()) NOT NULL,
    last_modified timestamp with time zone DEFAULT timezone('utc'::text, now()) NOT NULL
);


ALTER TABLE database_versions OWNER TO dbhub;

--
-- Name: database_versions_idnum_seq; Type: SEQUENCE; Schema: public; Owner: dbhub
--

CREATE SEQUENCE database_versions_idnum_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE database_versions_idnum_seq OWNER TO dbhub;

--
-- Name: database_versions_idnum_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: dbhub
--

ALTER SEQUENCE database_versions_idnum_seq OWNED BY database_versions.idnum;


--
-- Name: sqlite_databases; Type: TABLE; Schema: public; Owner: dbhub
--

CREATE TABLE sqlite_databases (
    username text NOT NULL,
    folder text NOT NULL,
    dbname text NOT NULL,
    public boolean NOT NULL DEFAULT false,
    date_created timestamp with time zone DEFAULT timezone('utc'::text, now()) NOT NULL,
    last_modified timestamp with time zone DEFAULT timezone('utc'::text, now()) NOT NULL,
    watchers bigint DEFAULT 0 NOT NULL,
    stars bigint DEFAULT 0 NOT NULL,
    forks bigint DEFAULT 0 NOT NULL,
    discussions bigint DEFAULT 0 NOT NULL,
    pull_requests bigint DEFAULT 0 NOT NULL,
    updates bigint DEFAULT 0 NOT NULL,
    branches bigint DEFAULT 1 NOT NULL,
    releases bigint DEFAULT 0 NOT NULL,
    contributors bigint DEFAULT 1 NOT NULL,
    description text,
    readme text,
    idnum integer NOT NULL,
    minio_bucket text NOT NULL,
    root_database integer,
    forked_from integer,
    default_table text
);


ALTER TABLE sqlite_databases OWNER TO dbhub;

--
-- Name: sqlite_databases_idnum_seq; Type: SEQUENCE; Schema: public; Owner: dbhub
--

CREATE SEQUENCE sqlite_databases_idnum_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE sqlite_databases_idnum_seq OWNER TO dbhub;

--
-- Name: sqlite_databases_idnum_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: dbhub
--

ALTER SEQUENCE sqlite_databases_idnum_seq OWNED BY sqlite_databases.idnum;


--
-- Name: users; Type: TABLE; Schema: public; Owner: dbhub
--

CREATE TABLE users (
    username text NOT NULL,
    date_joined timestamp with time zone DEFAULT timezone('utc'::text, now()),
    email text,
    client_certificate bytea NOT NULL,
    password_hash text NOT NULL,
    watchers bigint DEFAULT 0,
    minio_bucket text,
    pref_max_rows integer DEFAULT 10 NOT NULL,
    auth0id text
);


ALTER TABLE users OWNER TO dbhub;

--
-- Name: database_versions idnum; Type: DEFAULT; Schema: public; Owner: dbhub
--

ALTER TABLE ONLY database_versions ALTER COLUMN idnum SET DEFAULT nextval('database_versions_idnum_seq'::regclass);


--
-- Name: sqlite_databases idnum; Type: DEFAULT; Schema: public; Owner: dbhub
--

ALTER TABLE ONLY sqlite_databases ALTER COLUMN idnum SET DEFAULT nextval('sqlite_databases_idnum_seq'::regclass);


--
-- Name: database_versions database_versions_idnum_pkey; Type: CONSTRAINT; Schema: public; Owner: dbhub
--

ALTER TABLE ONLY database_versions
    ADD CONSTRAINT database_versions_idnum_pkey PRIMARY KEY (idnum);


--
-- Name: sqlite_databases sqlite_databases_idnum_key; Type: CONSTRAINT; Schema: public; Owner: dbhub
--

ALTER TABLE ONLY sqlite_databases
    ADD CONSTRAINT sqlite_databases_idnum_key PRIMARY KEY (idnum);


--
-- Name: sqlite_databases sqlite_databases_root_database_fkey; Type: CONSTRAINT; Schema: public; Owner: dbhub
--

ALTER TABLE sqlite_databases
  ADD CONSTRAINT sqlite_databases_root_database_fkey FOREIGN KEY (root_database) REFERENCES sqlite_databases (idnum) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: sqlite_databases sqlite_databases_forked_from_fkey; Type: CONSTRAINT; Schema: public; Owner: dbhub
--

ALTER TABLE sqlite_databases
  ADD CONSTRAINT sqlite_databases_forked_from_fkey FOREIGN KEY (forked_from) REFERENCES sqlite_databases (idnum) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: users users_minio_bucket_uniq; Type: CONSTRAINT; Schema: public; Owner: dbhub
--

ALTER TABLE ONLY users
    ADD CONSTRAINT users_minio_bucket_uniq UNIQUE (minio_bucket);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: dbhub
--

ALTER TABLE ONLY users
    ADD CONSTRAINT users_pkey PRIMARY KEY (username);


--
-- Name: database_stars_db_idx; Type: INDEX; Schema: public; Owner: dbhub
--

CREATE INDEX database_stars_db_idx ON database_stars USING btree (db);


--
-- Name: database_stars_user_idx; Type: INDEX; Schema: public; Owner: dbhub
--

CREATE INDEX database_stars_user_idx ON database_stars USING btree (username);


--
-- Name: database_versions_db_idx; Type: INDEX; Schema: public; Owner: dbhub
--

CREATE INDEX database_versions_db_idx ON database_versions USING btree (db);


--
-- Name: dbname_idx; Type: INDEX; Schema: public; Owner: dbhub
--

CREATE INDEX dbname_idx ON sqlite_databases USING btree (dbname);


--
-- Name: username_idx; Type: INDEX; Schema: public; Owner: dbhub
--

CREATE INDEX username_idx ON sqlite_databases USING btree (username);


--
-- Name: users_username_idx; Type: INDEX; Schema: public; Owner: dbhub
--

CREATE INDEX users_username_idx ON users USING btree (username);


--
-- Name: users_auth0id_idx; Type: INDEX; Schema: public; Owner: dbhub
--

CREATE INDEX users_auth0id_idx ON users USING btree (auth0id);



--
-- Name: database_stars database_stars_db_constraint; Type: FK CONSTRAINT; Schema: public; Owner: dbhub
--

ALTER TABLE ONLY database_stars
    ADD CONSTRAINT database_stars_db_constraint FOREIGN KEY (db) REFERENCES sqlite_databases(idnum) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_stars database_stars_user_constraint; Type: FK CONSTRAINT; Schema: public; Owner: dbhub
--

ALTER TABLE ONLY database_stars
    ADD CONSTRAINT database_stars_user_constraint FOREIGN KEY (username) REFERENCES users(username) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: database_versions database_versions_db_constraint; Type: FK CONSTRAINT; Schema: public; Owner: dbhub
--

ALTER TABLE ONLY database_versions
    ADD CONSTRAINT database_versions_db_constraint FOREIGN KEY (db) REFERENCES sqlite_databases(idnum) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: sqlite_databases sqlite_databases_minio_bucket_fkey; Type: FK CONSTRAINT; Schema: public; Owner: dbhub
--

ALTER TABLE ONLY sqlite_databases
    ADD CONSTRAINT sqlite_databases_minio_bucket_fkey FOREIGN KEY (minio_bucket) REFERENCES users(minio_bucket) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: sqlite_databases sqlite_databases_username_fkey; Type: FK CONSTRAINT; Schema: public; Owner: dbhub
--

ALTER TABLE ONLY sqlite_databases
    ADD CONSTRAINT sqlite_databases_username_fkey FOREIGN KEY (username) REFERENCES users(username) ON UPDATE CASCADE ON DELETE CASCADE;

