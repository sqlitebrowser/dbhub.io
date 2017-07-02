# Database schema for the DBHub applications

This creates an empty DBHub.io database (no users, SQLite databases,
etc), ready for the DBHub.io applications to use.

To load this into a fresh PostgreSQL 9.6 database run these commands
from the postgres superuser:

    $ createuser -d dbhub
    $ createdb -O dbhub dbhub
    $ psql < dbhub.sql

It should finish with no errors.

Note - This schema was created using:

    $ pg_dump -Os -U dbhub dbhub > dbhub.sql
