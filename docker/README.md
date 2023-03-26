## Description

This is a docker image for running the DBHub.io daemons, so we can test PR's
automatically.  And eventually, probably automatically test DB4S communication
with them too.

It includes the four DBHub.io daemons:

* The webUI, listening on port 9443
* The REST API end point, listening on port 9444
* The DB4S end point (the daemon DB Browser for SQLite talks to) on port 5550
* The internal-use-only "live" database daemon,

...and the dependencies for the daemons:

* PostgreSQL
* Memcached
* Minio
* RabbitMQ

This is done as an all-in-one image for now.  It _might_ be better separated
into separate services per damon (eg for docker-compose), but that'll be a
later thing (if needed).


## Server name

The (self signed) certificate authority and associated certs in the certs/
folder are designed for use by the docker image.  They have a (somewhat)
fixed name of "docker-dev.dbhub.io", which you'll need to add to your
local desktops /etc/hosts, pointing at the running docker image.

This way, when your web browser (Firefox, etc) tries to visit:

    https://docker-dev.dbhub.io:9443

... it will go to the docker image, using the name expected by the
server certificate.  Having the name match correctly is useful.

Or just use "localhost" as the server name, and tell your software not
to verify the certificate.  Either way works.
