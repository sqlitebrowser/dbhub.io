## Description

This is an in-development all-in-one docker image for local development
of the DBHub.io daemons, or development of DB Browser for SQLite's (DB4S)
communication with those daemons.

It includes the two DBHub.io daemons:

* The webUI, listening on port 8080
* The DB4S end point (the daemon DB Browser for SQLite talks to) on port 5550

...and the dependencies for the daemons:

* PostgreSQL
* Memcached
* Minio

This is done as an all-in-one image - which should work with all local docker
deployments - instead of as separate services, purely because it should be
easier to maintain this way. :)


## How to use

To build this image yourself, from this "docker" subdirectory type:

    $ docker build .

That should generate the image successfully, and give an image ID on the
final line.

To run the image, use that image ID with the following command:

    $ docker run -it --rm <image ID>

This will place you in an ash command shell, running as root in a container
of the image.  There are two scripts worth knowing about:

* /usr/local/bin/init.sh
* /usr/local/bin/start.sh

The init.sh initialises Minio and the PostgreSQL database, then loads the
DBHub.io schema into the database.

You'll want to run this before running start.sh, which starts all of the
daemons (Memcached, Minio, PostgreSQL, DBHub.io webUI, DBHub.io DB4S end
point.

If you want to keep the PostgreSQL and Minio data between sessions, you'll
need to mount the /data directory in the container to somewhere on your local
pc.  Do this by starting the docker container with this line instead:

    $ docker run -it --rm -v /some/diretory/on/your/pc:/data <image ID>

This time, when the init.sh script is run, it will create the PostgreSQL
database + Minio structures in that folder on your disk.  Using the same
location between sessions will persist the data across all of those
sessions.


## Server name

The (self signed) certificate authority and associated certs in the certs/
folder are designed for use by the docker image.  They have a (somewhat)
fixed name of "docker-dev.dbhub.io", which you'll need to add to your
local desktops /etc/hosts, pointing at the running docker image.

This way, when your web browser (Firefox, etc) tries to visit:

    https://docker-dev.dbhub.io

... it will go to the docker image, using the name expected by the
server certificate.  Having the name match correctly is useful.
