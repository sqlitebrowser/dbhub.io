
-- *** api_keys tables needed a unique key added to the key_id column ***

--
-- Name: api_keys api_keys_key_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_key_id UNIQUE (key_id);

-- *** Hang on, this ^^^ seems like it's just a constraint, but no index?  Look into it when less sleepy. ;)

    
-- *** New table for holding API key permissions ***

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
 
