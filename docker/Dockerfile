# vim:set ft=dockerfile:
FROM alpine:3.14

LABEL maintainer="Justin Clift <justin@postgresql.org>"

# Install Git, Go, Memcached, and PostgreSQL
RUN  \
    apk update && \
    apk upgrade && \
    apk add --no-cache ca-certificates 'curl>7.61.0' git go libc-dev memcached postgresql sqlite-dev

# Create the DBHub.io OS user
RUN addgroup dbhub && \
    adduser -D -S -s /bin/ash -G dbhub dbhub

### Memcached

# Generate script for starting Memcached
RUN echo "/usr/bin/memcached -u memcached -d" >> /usr/local/bin/start.sh && \
    chmod +x /usr/local/bin/start.sh

### Minio (everything except downloading the server executable)

# Create the Minio OS user
RUN addgroup minio && \
    adduser -D -S -s /bin/ash -G minio minio

# Install Minio
ENV MINIO_UPDATE off
ENV MINIO_ROOT_USER minio
ENV MINIO_ROOT_PASSWORD minio123
ENV MINIO_DATA /data/minio

# Add script pieces for initialising & starting Minio
RUN echo "mkdir -p ${MINIO_DATA}" >> /usr/local/bin/init.sh && \
    echo "chown minio:minio ${MINIO_DATA}" >> /usr/local/bin/init.sh && \
    su - minio -c "echo export MINIO_UPDATE=${MINIO_UPDATE} >> .profile" && \
    su - minio -c "echo export MINIO_ROOT_USER=${MINIO_ROOT_USER} >> .profile" && \
    su - minio -c "echo export MINIO_ROOT_PASSWORD=${MINIO_ROOT_PASSWORD} >> .profile" && \
    su - dbhub -c "echo export MINIO_ROOT_USER=${MINIO_ROOT_USER} >> .profile" && \
    su - dbhub -c "echo export MINIO_ROOT_PASSWORD=${MINIO_ROOT_PASSWORD} >> .profile" && \
    echo "su - minio -c '/usr/local/bin/minio server ${MINIO_DATA} &'" >> /usr/local/bin/start.sh

### PostgreSQL

# Create PostgreSQL directories
ENV PGDATA /data/postgresql
RUN su - postgres -c "echo export PGDATA=${PGDATA} >> .profile"
RUN mkdir -p "$PGDATA" && \
    chown -R postgres:postgres "$PGDATA" && \
    chmod 777 "$PGDATA" # this 777 will be replaced by 700 at runtime (allows semi-arbitrary "--user" values)
RUN mkdir /run/postgresql && \
    chown postgres:postgres /run/postgresql

# Add script pieces for initialising & starting PostgreSQL
RUN echo "mkdir -p ${PGDATA}" >> /usr/local/bin/init.sh && \
    echo "chown -R postgres:postgres ${PGDATA}" >> /usr/local/bin/init.sh && \
    echo "chmod 777 ${PGDATA}" >> /usr/local/bin/init.sh && \
    echo "su - postgres -c 'pg_ctl -D ${PGDATA} initdb'" >> /usr/local/bin/init.sh && \
    echo "su - postgres -c 'pg_ctl -D ${PGDATA} start'" >> /usr/local/bin/init.sh && \
    echo "su - postgres -c 'createuser -d dbhub'" >> /usr/local/bin/init.sh && \
    echo "su - postgres -c 'createdb -O dbhub dbhub'" >> /usr/local/bin/init.sh && \
    echo "su - dbhub -c 'psql dbhub < /go/src/github.com/sqlitebrowser/dbhub.io/database/dbhub.sql'" >> /usr/local/bin/init.sh && \
    echo "su - postgres -c 'pg_ctl -D ${PGDATA} stop'" >> /usr/local/bin/init.sh && \
    echo "su - postgres -c 'pg_ctl -D ${PGDATA} start'" >> /usr/local/bin/start.sh && \
    chmod +x /usr/local/bin/init.sh

# Set the dependencies and DBHub.io daemons to automatically start
ENTRYPOINT /usr/local/bin/start.sh

### DBHub.io (initial dir creation only)

# Create directores for the DBHub daemons
RUN mkdir -p /var/log/dbhub ~dbhub/.dbhub/disk_cache ~dbhub/.dbhub/email_queue && \
    chown -R dbhub:dbhub /var/log/dbhub ~dbhub/.dbhub/disk_cache ~dbhub/.dbhub/email_queue && \
    chmod 700 /var/log/dbhub ~dbhub/.dbhub/disk_cache ~dbhub/.dbhub/email_queue

### Other pieces

# Config file
ENV CONFIG_FILE /go/src/github.com/sqlitebrowser/dbhub.io/docker/config.toml

# Add script pieces for starting DBHub.io services
RUN echo "echo 127.0.0.1 docker-dev.dbhub.io docker-dev >> /etc/hosts" >> /usr/local/bin/start.sh && \
    echo "su - dbhub -c 'CONFIG_FILE=${CONFIG_FILE} /usr/local/bin/dbhub-webui &'" >> /usr/local/bin/start.sh && \
    echo "su - dbhub -c 'CONFIG_FILE=${CONFIG_FILE} /usr/local/bin/dbhub-db4s &'" >> /usr/local/bin/start.sh && \
    echo "su - dbhub -c 'CONFIG_FILE=${CONFIG_FILE} /usr/local/bin/dbhub-api'" >> /usr/local/bin/start.sh

### Minio (download the server executable)

# Download the latest Minio binary
RUN curl -L -o /usr/local/bin/minio https://dl.minio.io/server/minio/release/linux-amd64/minio && \
    chmod +x /usr/local/bin/minio

# Build the DBHub.io daemons
RUN mkdir -p /go/src/github.com/sqlitebrowser && \
    cd /go/src/github.com/sqlitebrowser && \
    git clone https://github.com/sqlitebrowser/dbhub.io && \
    cd /go/src/github.com/sqlitebrowser/dbhub.io &&  \
    go build -gcflags "all=-N -l" -o /usr/local/bin/dbhub-api github.com/sqlitebrowser/dbhub.io/api && \
    go build -gcflags "all=-N -l" -o /usr/local/bin/dbhub-db4s github.com/sqlitebrowser/dbhub.io/db4s && \
    go build -gcflags "all=-N -l" -o /usr/local/bin/dbhub-webui github.com/sqlitebrowser/dbhub.io/webui

# Initialise everything
RUN /usr/local/bin/init.sh
