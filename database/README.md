# Database schema for the DBHub applications

[**NOTE - DO NOT update the schema file, as we're using migrations
to make changes now**]

This creates an empty DBHub.io database (no users, SQLite databases,
etc), ready for the DBHub.io applications to use.

To load this into a fresh PostgreSQL database, run these commands
from the postgres superuser:

    $ createuser -d dbhub
    $ createdb -O dbhub dbhub
    $ psql dbhub < dbhub.sql

It should finish with no errors.

Note - This schema was created using:

    $ pg_dump -Os -U dbhub dbhub > dbhub.sql

## Migrations

To install the `migrate` cli locally on your system, use:

    $ go install -tags 'pgx5' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

This will build `migrate` with the driver we use (`pgx5`), then install
it to your `$GOBIN` directory.  If you don't have `GOBIN` defined, it'll
go into `$HOME/go/bin/` instead.

## Manually running the migrations

In theory (!), you shouldn't need to apply the migrations manually as
that's automatically done by our daemons when any of them start.

But, this is how to apply them in your local (DBHub.io) Docker container
database if needed for some reason:

    $ migrate -database pgx5://dbhub@localhost:5432/dbhub -path database/migrations up
