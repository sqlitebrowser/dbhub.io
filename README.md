## DBHub.io

[![Cypress](https://github.com/sqlitebrowser/dbhub.io/actions/workflows/cypress.yml/badge.svg)](https://github.com/sqlitebrowser/dbhub.io/actions/workflows/cypress.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/sqlitebrowser/dbhub.io)](https://goreportcard.com/report/github.com/sqlitebrowser/dbhub.io)


### What it is

A Cloud for SQLite databases, with special integration for [DB Browser for SQLite](http://sqlitebrowser.org).

You can use our live hosted version at [https://dbhub.io](https://dbhub.io/justinclift/DB4S%20download%20stats.sqlite),
our API server at https://api.dbhub.io, or run things locally for your own users.

### Screenshot

<img src="https://github.com/sqlitebrowser/db4s-screenshots/raw/master/dbhub/2017-08-10/00-database_view_page.png" alt="DBHub.io Screenshot" align="middle" width="550px" />

### Requirements

* [Golang](https://golang.org) - version 1.18 or above is required.
* [Memcached](https://memcached.org) - version 1.4.33 and above are known to work.
* [Minio](https://minio.io) - release 2016-11-26T02:23:47Z and later are known to work.
* [NodeJS](https://nodejs.org) - version 20 is known to work, others are untested.
* [PostgreSQL](https://www.postgresql.org) - version 13 and above are known to work.
* [Yarn](https://classic.yarnpkg.com) - version 1.22.x.  Not Yarn 2.x or greater.

### Subdirectories

* [api](api/) - A very simple API server, used for querying databases remotely.
* [common](common/) - Library of functions used by the DBHub.io components.
* [database](database/) - PostgreSQL database schema.
* [default_licences](default_licences/) - Useful Open Source licences suitable for databases.
* [db4s](db4s/) - REST server which [DB Browser for SQLite](http://sqlitebrowser.org)
  and [Dio](https://github.com/sqlitebrowser/dio) use for communicating with DBHub.io.
* [live](live/) - Internal daemon which manages live SQLite databases.
* [webui](webui/) - The main public facing webUI.

### Libraries for accessing DBHub.io via API

* [go-dbhub](https://github.com/sqlitebrowser/go-dbhub) - A Go library for accessing and using your SQLite libraries on DBHub.io.
* [pydbhub](https://github.com/LeMoussel/pydbhub) - A Python library for accessing and using your SQLite libraries on DBHub.io.
