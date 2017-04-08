## DBHub.io

### What it is

An early stage (in development) "Cloud" for SQLite databases, with special
integration for [DB Browser for SQLite](http://sqlitebrowser.org).

You can try out our development server at [https://dev1.dbhub.io](https://dev1.dbhub.io),
or run it locally for your own users.

**Note** - Don't store important data on the dev1 server.  It gets wiped now and again,
when we're experimenting with things.

When the core code is more fully featured (June/July 2017 target), we'll start putting
"production" servers online for people to store their data.

### Requirements

* [Golang](https://golang.org) - version 1.8 and above are known to work.
* [Memcached](https://memcached.org) - version 1.4.33 and above are known to work.
* [Minio](https://minio.io) - release 2016-11-26T02:23:47Z and later are known to work.
* [PostgreSQL](https://www.postgresql.org) - version 9.5 and above are known to work.

### Subdirectories

* [admin](admin/) - Internal only (not public facing) webUI for admin tasks
* [common](common/) - Library of functions used by the DBHub.io components
* [database](database/) - PostgreSQL database schema
* [db4s](db4s/) - REST server which [DB Browser for SQLite](http://sqlitebrowser.org)
  connects to with File → Remote
* [webui](webui/) - The main public facing webUI
