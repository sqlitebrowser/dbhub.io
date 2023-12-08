# vim:set ft=dockerfile:

# We grab RabbitMQ and Erlang from the official RabbitMQ image
FROM rabbitmq:3-alpine AS rabbit

# Build our own image
FROM alpine:3.19

LABEL maintainer="Justin Clift <justin@postgresql.org>"

# Copy the RabbitMQ files we need
COPY --from=rabbit /etc/rabbitmq /etc/rabbitmq
COPY --from=rabbit /opt/erlang /opt/erlang
COPY --from=rabbit /opt/openssl /opt/openssl
COPY --from=rabbit /opt/rabbitmq /opt/rabbitmq
COPY --from=rabbit /var/lib/rabbitmq /var/lib/rabbitmq
COPY --from=rabbit /var/log/rabbitmq /var/log/rabbitmq

# Create the rabbitmq user and group
RUN addgroup -S rabbitmq && \
    adduser -S -D -h /var/lib/rabbitmq -s /sbin/nologin -g "Linux User,,," -G rabbitmq rabbitmq && \
    chown -Rh rabbitmq: /opt/rabbitmq /var/log/rabbitmq

# Use a fast Australian mirror for the Alpine package repositories
# Without doing this, building the image can take 2+ hours. :(
RUN echo "https://mirror.aarnet.edu.au/pub/alpine/v3.19/main" > /etc/apk/repositories && \
    echo "https://mirror.aarnet.edu.au/pub/alpine/v3.19/community" >> /etc/apk/repositories

# Install Git, Go, Memcached, Minio, and PostgreSQL
RUN apk update && \
    apk upgrade && \
    apk add --no-cache bison ca-certificates 'curl>7.61.0' file flex git go libc-dev make memcached minio openssl openssl-dev postgresql postgresql-dev shadow yarn

# Add PostgreSQL jsquery extension
RUN mkdir /install && \
    cd /install && \
    git clone https://github.com/postgrespro/jsquery.git && \
    cd jsquery && \
    make USE_PGXS=1 && \
    make USE_PGXS=1 install

# Create the DBHub.io OS user
RUN addgroup dbhub && \
    adduser -D -S -s /bin/ash -G dbhub dbhub

# Location of the DBHub.io source code
ENV DBHUB_SOURCE /dbhub.io

# Set Minio config variables
ENV MINIO_ROOT_USER minio
ENV MINIO_ROOT_PASSWORD minio123

# Run each of our (non RabbitMQ) daemon dependencies at least once to ensure they initialise ok, and populate the DBHub.io database
RUN echo "echo export PGDATA=/var/lib/postgresql/data > ~postgres/.profile" >> /usr/local/bin/init.sh && \
    echo "echo export MINIO_ROOT_USER=${MINIO_ROOT_USER} > ~minio/.profile" >> /usr/local/bin/init.sh && \
    echo "echo export MINIO_ROOT_PASSWORD=${MINIO_ROOT_PASSWORD} >> ~minio/.profile" >> /usr/local/bin/init.sh && \
    echo "su - postgres -c '/usr/libexec/postgresql/initdb --locale=en_US.UTF-8'" >> /usr/local/bin/init.sh && \
    echo "mkdir /run/postgresql /home/memcached" >> /usr/local/bin/init.sh && \
    echo "chown memcached: /home/memcached" >> /usr/local/bin/init.sh && \
    echo "chown -R minio: /var/lib/minio" >> /usr/local/bin/init.sh && \
    echo "chown -R postgres: /var/lib/postgresql /run/postgresql" >> /usr/local/bin/init.sh && \
    echo "usermod -s /bin/sh memcached" >> /usr/local/bin/init.sh && \
    echo "usermod -s /bin/sh minio" >> /usr/local/bin/init.sh && \
    echo "su - postgres -c '/usr/libexec/postgresql/pg_ctl start'" >> /usr/local/bin/init.sh && \
    echo "createuser -U postgres -ds dbhub" >> /usr/local/bin/init.sh && \
    echo "createdb -U postgres -O dbhub dbhub" >> /usr/local/bin/init.sh && \
    echo "su - dbhub -c 'psql dbhub < ${DBHUB_SOURCE}/database/dbhub.sql'" >> /usr/local/bin/init.sh && \
    echo "su - memcached -c '/usr/bin/memcached -d'" >> /usr/local/bin/init.sh && \
    echo "su - minio -c '/usr/bin/minio server --quiet --anonymous /var/lib/minio/data 2>&1 &'" >> /usr/local/bin/init.sh && \
    echo "su - postgres -c '/usr/libexec/postgresql/pg_ctl stop' 2>&1 | grep -v 'Read-only file system'" >> /usr/local/bin/init.sh && \
    chmod +x /usr/local/bin/init.sh

# Set the dependencies and DBHub.io daemons to automatically start
ENTRYPOINT /usr/local/bin/start.sh

# Create directores for the DBHub daemons
RUN mkdir -p /var/log/dbhub ~dbhub/.dbhub/disk_cache && \
    chown -R dbhub:dbhub /var/log/dbhub ~dbhub/.dbhub/disk_cache && \
    chmod 700 /var/log/dbhub ~dbhub/.dbhub/disk_cache

# DBHub.io config file
ENV CONFIG_FILE ${DBHUB_SOURCE}/docker/config.toml

# Build the Go debugger (Delve)
RUN GOBIN=/usr/local/bin go install github.com/go-delve/delve/cmd/dlv@latest

# Add script pieces for starting DBHub.io services
RUN echo "echo 127.0.0.1 docker-dev.dbhub.io docker-dev >> /etc/hosts" >> /usr/local/bin/start.sh && \
    echo "echo nameserver 8.8.8.8 > /etc/resolv.conf" >> /usr/local/bin/start.sh && \
    echo "su - memcached -c '/usr/bin/memcached -d'" >> /usr/local/bin/start.sh && \
    echo "su - minio -c '/usr/bin/minio server --quiet --anonymous /var/lib/minio/data 2>&1 &'" >> /usr/local/bin/start.sh && \
    echo "su - postgres -c '/usr/libexec/postgresql/pg_ctl start'" >> /usr/local/bin/start.sh && \
    echo "" >> /usr/local/bin/start.sh && \
    echo "unset CONFIG_FILE" >> /usr/local/bin/start.sh && \
    echo "export RABBITMQ_CONFIG_FILES=/etc/rabbitmq/conf.d" >> /usr/local/bin/start.sh && \
    echo "export PATH=/opt/rabbitmq/sbin:/opt/erlang/bin:/opt/openssl/bin:$PATH" >> /usr/local/bin/start.sh && \
    echo "/opt/rabbitmq/sbin/rabbitmq-server &" >> /usr/local/bin/start.sh && \
    echo "" >> /usr/local/bin/start.sh && \
    echo "# Wait for RabbitMQ to start before launching the DBHub.io daemons" >> /usr/local/bin/start.sh && \
    echo "sleep 15" >> /usr/local/bin/start.sh && \
    echo "" >> /usr/local/bin/start.sh && \
    echo "su - dbhub -c 'if [ -f "${DBHUB_SOURCE}/.env" ]; then source ${DBHUB_SOURCE}/.env; fi; CONFIG_FILE=${CONFIG_FILE} /usr/local/bin/dbhub-webui >>/home/dbhub/output.log 2>&1 &'" >> /usr/local/bin/start.sh && \
    echo "su - dbhub -c 'if [ -f "${DBHUB_SOURCE}/.env" ]; then source ${DBHUB_SOURCE}/.env; fi; CONFIG_FILE=${CONFIG_FILE} /usr/local/bin/dbhub-api >>/home/dbhub/output.log 2>&1 &'" >> /usr/local/bin/start.sh && \
    echo "su - dbhub -c 'if [ -f "${DBHUB_SOURCE}/.env" ]; then source ${DBHUB_SOURCE}/.env; fi; CONFIG_FILE=${CONFIG_FILE} /usr/local/bin/dbhub-db4s >>/home/dbhub/output.log 2>&1 &'" >> /usr/local/bin/start.sh && \
    echo "su - dbhub -c 'if [ -f "${DBHUB_SOURCE}/.env" ]; then source ${DBHUB_SOURCE}/.env; fi; CONFIG_FILE=${CONFIG_FILE} /usr/local/bin/dbhub-live node1 /tmp/node1 >>/home/dbhub/output.log 2>&1 &'" >> /usr/local/bin/start.sh && \
    echo "su - dbhub -c 'if [ -f "${DBHUB_SOURCE}/.env" ]; then source ${DBHUB_SOURCE}/.env; fi; CONFIG_FILE=${CONFIG_FILE} /usr/local/bin/dbhub-live node2 /tmp/node2 >>/home/dbhub/output.log 2>&1 &'" >> /usr/local/bin/start.sh && \
    echo "while :; do" >> /usr/local/bin/start.sh && \
    echo "  sleep 5" >> /usr/local/bin/start.sh && \
    echo "done" >> /usr/local/bin/start.sh && \
    chmod +x /usr/local/bin/start.sh

# Compile our own customised version of SQLite
RUN echo "Downloading SQLite source code" && \
    mkdir /sqlite && \
    cd /sqlite && \
    TARBALL=$(curl -s https://sqlite.org/download.html | awk '/<!--/,/-->/ {print}' | grep 'sqlite-autoconf' | cut -d ',' -f 3) && \
    SHA3=$(curl -s https://sqlite.org/download.html | awk '/<!--/,/-->/ {print}' | grep 'sqlite-autoconf' | cut -d ',' -f 5) && \
    curl -LsS -o sqlite.tar.gz https://sqlite.org/${TARBALL} && \
    VERIFY=$(openssl dgst -sha3-256 sqlite.tar.gz | cut -d ' ' -f 2) && \
    if [ "$SHA3" != "$VERIFY" ]; then exit 1 ; fi && \
    if [ ! -f sqlite.tar.gz ]; then echo "Downloading the SQLite source code did not work" ; exit 3 ; fi && \
    echo "Compiling local SQLite" && \
    tar xfz sqlite.tar.gz && \
    cd sqlite-autoconf-* || exit 4 && \
    CPPFLAGS="-DSQLITE_ENABLE_COLUMN_METADATA=1 -DSQLITE_MAX_VARIABLE_NUMBER=250000 -DSQLITE_ENABLE_RTREE=1 -DSQLITE_ENABLE_GEOPOLY=1 -DSQLITE_ENABLE_FTS3=1 -DSQLITE_ENABLE_FTS3_PARENTHESIS=1 -DSQLITE_ENABLE_FTS5=1 -DSQLITE_ENABLE_STAT4=1 -DSQLITE_ENABLE_JSON1=1 -DSQLITE_SOUNDEX=1 -DSQLITE_ENABLE_MATH_FUNCTIONS=1 -DSQLITE_MAX_ATTACHED=125 -DSQLITE_ENABLE_MEMORY_MANAGEMENT=1 -DSQLITE_ENABLE_SNAPSHOT=1" ./configure --prefix=/sqlite --enable-dynamic-extensions=no && \
    make -j9 && \
    make install && \
    cd .. && \
    rm -rf sqlite-autoconf-* && \
    echo "/sqlite/lib:/lib:/usr/local/lib:/usr/lib" > /etc/ld-musl-x86_64.path

# Create script to compile DBHub.io daemons
RUN echo "cd ${DBHUB_SOURCE}" >> /usr/local/bin/compile.sh && \
    echo "yarn" >> /usr/local/bin/compile.sh && \
    echo "yarn run babel ${DBHUB_SOURCE}/webui/jsx --out-dir ${DBHUB_SOURCE}/webui/js --presets babel-preset-react-app/prod" >> /usr/local/bin/compile.sh && \
    echo "yarn run webpack -c ${DBHUB_SOURCE}/webui/webpack.config.js" >> /usr/local/bin/compile.sh && \
    echo "cd ${DBHUB_SOURCE}/api" >> /usr/local/bin/compile.sh && \
    echo "PKG_CONFIG_PATH=/sqlite/lib/pkgconfig go build -gcflags \"all=-N -l\" -buildvcs=false -o /usr/local/bin/dbhub-api ." >> /usr/local/bin/compile.sh && \
    echo "cd ${DBHUB_SOURCE}/db4s" >> /usr/local/bin/compile.sh && \
    echo "PKG_CONFIG_PATH=/sqlite/lib/pkgconfig go build -gcflags \"all=-N -l\" -buildvcs=false -o /usr/local/bin/dbhub-db4s ." >> /usr/local/bin/compile.sh && \
    echo "cd ${DBHUB_SOURCE}/live" >> /usr/local/bin/compile.sh && \
    echo "PKG_CONFIG_PATH=/sqlite/lib/pkgconfig go build -gcflags \"all=-N -l\" -buildvcs=false -o /usr/local/bin/dbhub-live ." >> /usr/local/bin/compile.sh && \
    echo "cd ${DBHUB_SOURCE}/standalone/analysis" >> /usr/local/bin/compile.sh && \
    echo "PKG_CONFIG_PATH=/sqlite/lib/pkgconfig go build -gcflags \"all=-N -l\" -buildvcs=false -o /usr/local/bin/dbhub-analysis ." >> /usr/local/bin/compile.sh && \
    echo "ln -f -s /usr/local/bin/dbhub-analysis  /etc/periodic/15min/" >> /usr/local/bin/compile.sh && \
    echo "cd ${DBHUB_SOURCE}/webui" >> /usr/local/bin/compile.sh && \
    echo "PKG_CONFIG_PATH=/sqlite/lib/pkgconfig go build -gcflags \"all=-N -l\" -buildvcs=false -o /usr/local/bin/dbhub-webui ." >> /usr/local/bin/compile.sh && \
    echo "/usr/local/bin/restart.sh" >> /usr/local/bin/compile.sh && \
    chmod +x /usr/local/bin/compile.sh

# Create script to restart the DBHub.io webui and api daemons
RUN echo "# Kill the existing running daemons" >> /usr/local/bin/restart.sh && \
    echo "pkill dbhub-webui" >> /usr/local/bin/restart.sh && \
    echo "pkill dbhub-api" >> /usr/local/bin/restart.sh && \
    echo "pkill dbhub-db4s" >> /usr/local/bin/restart.sh && \
    echo "pkill dbhub-live" >> /usr/local/bin/restart.sh && \
    echo "pkill dlv" >> /usr/local/bin/restart.sh && \
    echo "" >> /usr/local/bin/restart.sh && \
    echo "# Restart the daemons" >> /usr/local/bin/restart.sh && \
    echo "chown dbhub: /home/dbhub/.dbhub" >> /usr/local/bin/restart.sh && \
    echo "if [ -c /dev/console ]; then" >> /usr/local/bin/restart.sh && \
    echo "  chmod o+w /dev/console" >> /usr/local/bin/restart.sh && \
    echo "fi" >> /usr/local/bin/restart.sh && \
    echo "su - dbhub -c 'if [ -f "${DBHUB_SOURCE}/.env" ]; then source ${DBHUB_SOURCE}/.env; fi; CONFIG_FILE=${CONFIG_FILE} nohup /usr/local/bin/dbhub-webui >>/home/dbhub/output.log 2>&1 &'" >> /usr/local/bin/restart.sh && \
    echo "su - dbhub -c 'if [ -f "${DBHUB_SOURCE}/.env" ]; then source ${DBHUB_SOURCE}/.env; fi; CONFIG_FILE=${CONFIG_FILE} nohup /usr/local/bin/dbhub-api >>/home/dbhub/output.log 2>&1 &'" >> /usr/local/bin/restart.sh && \
    echo "su - dbhub -c 'if [ -f "${DBHUB_SOURCE}/.env" ]; then source ${DBHUB_SOURCE}/.env; fi; CONFIG_FILE=${CONFIG_FILE} nohup /usr/local/bin/dbhub-db4s >>/home/dbhub/output.log 2>&1 &'" >> /usr/local/bin/restart.sh && \
    echo "su - dbhub -c 'if [ -f "${DBHUB_SOURCE}/.env" ]; then source ${DBHUB_SOURCE}/.env; fi; CONFIG_FILE=${CONFIG_FILE} nohup /usr/local/bin/dbhub-live node1 /tmp/node1 >>/home/dbhub/output.log 2>&1 &'" >> /usr/local/bin/restart.sh && \
    echo "su - dbhub -c 'if [ -f "${DBHUB_SOURCE}/.env" ]; then source ${DBHUB_SOURCE}/.env; fi; CONFIG_FILE=${CONFIG_FILE} nohup /usr/local/bin/dbhub-live node2 /tmp/node2 >>/home/dbhub/output.log 2>&1 &'" >> /usr/local/bin/restart.sh && \
    echo "" >> /usr/local/bin/restart.sh && \
    echo "# Delay long enough for the DBHub.io daemons to start" >> /usr/local/bin/restart.sh && \
    echo "sleep 1" >> /usr/local/bin/restart.sh && \
    chmod +x /usr/local/bin/restart.sh

# Create script to start the DBHub.io webui and api daemons using the Go debugger (Delve)
RUN echo "# Kill the existing running daemons" >> /usr/local/bin/debug.sh && \
    echo "pkill dbhub-webui" >> /usr/local/bin/debug.sh && \
    echo "pkill dbhub-api" >> /usr/local/bin/debug.sh && \
    echo "pkill dbhub-db4s" >> /usr/local/bin/debug.sh && \
    echo "pkill dbhub-live" >> /usr/local/bin/debug.sh && \
    echo "pkill dlv" >> /usr/local/bin/debug.sh && \
    echo "" >> /usr/local/bin/debug.sh && \
    echo "# Restart the daemons" >> /usr/local/bin/debug.sh && \
    echo "chown dbhub: /home/dbhub/.dbhub" >> /usr/local/bin/debug.sh && \
    echo "if [ -c /dev/console ]; then" >> /usr/local/bin/debug.sh && \
    echo "  chmod o+w /dev/console" >> /usr/local/bin/debug.sh && \
    echo "fi" >> /usr/local/bin/debug.sh && \
    echo "su - dbhub -c 'if [ -f "${DBHUB_SOURCE}/.env" ]; then source ${DBHUB_SOURCE}/.env; fi; CONFIG_FILE=${CONFIG_FILE} nohup dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec /usr/local/bin/dbhub-webui >>/home/dbhub/output.log 2>&1 &'" >> /usr/local/bin/debug.sh && \
    echo "su - dbhub -c 'if [ -f "${DBHUB_SOURCE}/.env" ]; then source ${DBHUB_SOURCE}/.env; fi; CONFIG_FILE=${CONFIG_FILE} nohup dlv --listen=:2346 --headless=true --api-version=2 --accept-multiclient exec /usr/local/bin/dbhub-api >>/home/dbhub/output.log 2>&1 &'" >> /usr/local/bin/debug.sh && \
    echo "su - dbhub -c 'if [ -f "${DBHUB_SOURCE}/.env" ]; then source ${DBHUB_SOURCE}/.env; fi; CONFIG_FILE=${CONFIG_FILE} nohup dlv --listen=:2347 --headless=true --api-version=2 --accept-multiclient exec /usr/local/bin/dbhub-db4s >>/home/dbhub/output.log 2>&1 &'" >> /usr/local/bin/debug.sh && \
    echo "su - dbhub -c 'if [ -f "${DBHUB_SOURCE}/.env" ]; then source ${DBHUB_SOURCE}/.env; fi; CONFIG_FILE=${CONFIG_FILE} nohup dlv --listen=:2348 --headless=true --api-version=2 --accept-multiclient exec /usr/local/bin/dbhub-live node1 /tmp/node1>>/home/dbhub/output.log 2>&1 &'" >> /usr/local/bin/debug.sh && \
    echo "su - dbhub -c 'if [ -f "${DBHUB_SOURCE}/.env" ]; then source ${DBHUB_SOURCE}/.env; fi; CONFIG_FILE=${CONFIG_FILE} nohup dlv --listen=:2349 --headless=true --api-version=2 --accept-multiclient exec /usr/local/bin/dbhub-live node2 /tmp/node2>>/home/dbhub/output.log 2>&1 &'" >> /usr/local/bin/debug.sh && \
    echo "" >> /usr/local/bin/debug.sh && \
    echo "# Delay long enough for the DBHub.io daemons to start" >> /usr/local/bin/debug.sh && \
    echo "sleep 1" >> /usr/local/bin/debug.sh && \
    chmod +x /usr/local/bin/debug.sh

# Build the DBHub.io daemons
RUN cd / && git clone --branch master --depth 5 https://github.com/sqlitebrowser/dbhub.io
RUN /usr/local/bin/compile.sh

# Populate the DBHub.io database
WORKDIR /usr/local/bin
RUN init.sh
