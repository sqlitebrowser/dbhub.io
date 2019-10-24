## 3DHub.io

### What it is

An early stage (in development) "Cloud" for 3D projects and models.

You can try out our live development server at [https://3dhub.io](https://3dhub.io), or run it locally for your own
users.

**Note** - Don't store important data on the live server.  We're still changing things
around on it quite often, and haven't yet put time into setting up good backups (etc).

When the core code is more fully featured (end of November 2019 target), we'll start
putting "production" servers online for people to store their projects.

<!-- 
### Screenshot

<img src="https://github.com/sqlitebrowser/db4s-screenshots/raw/master/dbhub/2017-08-10/00-database_view_page.png" alt="3DHub.io Screenshot" align="middle" width="550px" />

-->

### Requirements

* [Golang](https://golang.org) - development uses version 1.12.  Earlier versions may work, but are untested.
* [Memcached](https://memcached.org) - version 1.4.33 and above are known to work.
* [Minio](https://minio.io) - release 2016-11-26T02:23:47Z and later are known to work.
* [PostgreSQL](https://www.postgresql.org) - version 9.6 or above is required.

### Subdirectories

* [common](common/) - Library of functions used by the 3DHub.io components.
* [database](database/) - PostgreSQL database schema.
* [default_licences](default_licences/) - Useful Open Source licences suitable for databases.
  [Dio](https://github.com/sqlitebrowser/dio) Used for communicating with 3DHub.io.
* [webui](webui/) - The main public facing webUI.

