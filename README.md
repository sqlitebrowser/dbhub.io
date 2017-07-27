## DBHub.io

### What it is

An early stage (in development) "Cloud" for SQLite databases, with special
integration for [DB Browser for SQLite](http://sqlitebrowser.org).

You can try out our beta testing server at [https://db4s-beta.dbhub.io](https://db4s-beta.dbhub.io/justinclift/DB4S%20download%20stats.sqlite),
or run it locally for your own users.

**Note** - Don't store important data on the beta server.  It gets wiped now and again,
when we're experimenting with things.

When the core code is more fully featured (August 2017 target), we'll start putting
"production" servers online for people to store their data.

### Requirements

* [Golang](https://golang.org) - version 1.8 or above is required.
* [Memcached](https://memcached.org) - version 1.4.33 and above are known to work.
* [Minio](https://minio.io) - release 2016-11-26T02:23:47Z and later are known to work.
* [PostgreSQL](https://www.postgresql.org) - version 9.6 or above is required.

### Subdirectories

* [api](api/) - (not yet created) Future home for our JSON API server.
* [common](common/) - Library of functions used by the DBHub.io components.
* [database](database/) - PostgreSQL database schema.
* [default_licences](default_licences/) - Useful Open Source licences suitable for databases.
* [db4s](db4s/) - REST server which [DB Browser for SQLite](http://sqlitebrowser.org)
  connects to with File → Remote.
* [dio](dio/) - (not yet created) Future home for `dio`, our command line interface (cli) for
  interacting with DBHub.io.
* [webui](webui/) - The main public facing webUI.

## Related mailing lists

### Announcements

A low volume announcements only mailing list:

* https://lists.sqlitebrowser.org/mailman/listinfo/dbhub-announce

### Developers

For development related discussion about DB4S and DBHub.io:

* https://lists.sqlitebrowser.org/mailman/listinfo/db4s-dev
