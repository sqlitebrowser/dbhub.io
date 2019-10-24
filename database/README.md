# Database schema for the 3DHub applications

This creates an empty 3DHub.io database (no users, projects, etc),
ready for the 3DHub.io applications to use.

To load this into a fresh PostgreSQL 9.6 (or higher) database, run
these commands from a postgres superuser:

    $ createuser -d 3dhub
    $ createdb -O 3dhub 3dhub
    $ psql 3dhub < schema.sql

It should finish with no errors.

Note - This schema is created using:

    $ pg_dump -Os -U 3dhub 3dhub > schema.sql
