## DBHub.io

### What it is

An early stage (in development) "Cloud" for SQLite databases, with special
integration for [DB Browser for SQLite](http://sqlitebrowser.org).

You can try out our live server at [https://dbhub.io](https://db4s.dbhub.io/justinclift/DB4S%20download%20stats.sqlite),
or run it locally for your own users.

### Screenshot

<img src="https://github.com/sqlitebrowser/db4s-screenshots/raw/master/dbhub/2017-08-10/00-database_view_page.png" alt="DBHub.io Screenshot" align="middle" width="550px" />

### Requirements

* [Golang](https://golang.org) - version 1.8 or above is required, with 1.9 highly recommended.
* [Memcached](https://memcached.org) - version 1.4.33 and above are known to work.
* [Minio](https://minio.io) - release 2016-11-26T02:23:47Z and later are known to work.
* [PostgreSQL](https://www.postgresql.org) - version 9.6 or above is required.

### Subdirectories

* [api](api/) - A very simple API server, used for querying databases remotely.
* [common](common/) - Library of functions used by the DBHub.io components.
* [database](database/) - PostgreSQL database schema.
* [default_licences](default_licences/) - Useful Open Source licences suitable for databases.
* [db4s](db4s/) - REST server which [DB Browser for SQLite](http://sqlitebrowser.org)
  and [Dio](https://github.com/sqlitebrowser/dio) use for communicating with DBHub.io.
* [webui](webui/) - The main public facing webUI.

