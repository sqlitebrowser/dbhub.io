# vim:set ft=dockerfile:
FROM alpine:3.17

LABEL maintainer="Justin Clift <justin@postgresql.org>"

# Install Git, Go, Memcached, Minio, OpenRC, PostgreSQL, and RabbitMQ
RUN  \
    apk update && \
    apk upgrade && \
    apk add --no-cache ca-certificates 'curl>7.61.0' git go libc-dev memcached minio openrc postgresql sqlite-dev yarn && \
    apk add rabbitmq-server --repository http://dl-cdn.alpinelinux.org/alpine/edge/testing/ && \
    rc-update add memcached default && \
    rc-update add minio default && \
    rc-update add postgresql default && \
    rc-update add rabbitmq-server default && \
    mkdir /etc/rabbitmq && \
    rabbitmq-plugins enable rabbitmq_management && \
    rabbitmq-plugins enable rabbitmq_top

# Create the DBHub.io OS user
RUN addgroup dbhub && \
    adduser -D -S -s /bin/ash -G dbhub dbhub

# Location of the DBHub.io source code
ENV DBHUB_SOURCE /dbhub.io

# Set Minio config variables
ENV MINIO_ROOT_USER minio
ENV MINIO_ROOT_PASSWORD minio123

RUN sed -i "s/^MINIO_ROOT_USER=\"change-me\"/MINIO_ROOT_USER=\"${MINIO_ROOT_USER}\"/" /etc/conf.d/minio && \
    sed -i "s/^MINIO_ROOT_PASSWORD=\"change-me\"/MINIO_ROOT_PASSWORD=\"${MINIO_ROOT_PASSWORD}\"/" /etc/conf.d/minio

# Run each of our daemon dependencies at least once to ensure they initialise ok, and populate the DBHub.io database
RUN echo "openrc nonetwork" >> /usr/local/bin/init.sh && \
    echo "openrc default stop 2>&1 | grep -v 'Read-only file system'" >> /usr/local/bin/init.sh && \
    echo "createuser -U postgres -d dbhub" >> /usr/local/bin/init.sh && \
    echo "createdb -U postgres -O dbhub dbhub" >> /usr/local/bin/init.sh && \
    echo "su - dbhub -c 'psql dbhub < ${DBHUB_SOURCE}/database/dbhub.sql'" >> /usr/local/bin/init.sh && \
    echo "rc-service memcached stop 2>&1 | grep -v 'Read-only file system'" >> /usr/local/bin/init.sh && \
    echo "rc-service minio stop 2>&1 | grep -v 'Read-only file system'" >> /usr/local/bin/init.sh && \
    echo "rc-service postgresql stop 2>&1 | grep -v 'Read-only file system'" >> /usr/local/bin/init.sh && \
    echo "rc-service rabbitmq-server stop 2>&1 | grep -v 'Read-only file system'" >> /usr/local/bin/init.sh && \
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
# These don't use openrc.  Not sure if it'd be useful.  Maybe a task for a different day?
RUN echo "echo 127.0.0.1 docker-dev.dbhub.io docker-dev >> /etc/hosts" >> /usr/local/bin/start.sh && \
    echo "openrc default" >> /usr/local/bin/start.sh && \
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

# Create script to compile DBHub.io daemons
RUN echo "cd ${DBHUB_SOURCE}" >> /usr/local/bin/compile.sh && \
    echo "yarn" >> /usr/local/bin/compile.sh && \
    echo "yarn run babel ${DBHUB_SOURCE}/webui/jsx --out-dir ${DBHUB_SOURCE}/webui/js --presets babel-preset-react-app/prod" >> /usr/local/bin/compile.sh && \
    echo "yarn run webpack -c ${DBHUB_SOURCE}/webui/webpack.config.js" >> /usr/local/bin/compile.sh && \
    echo "cd ${DBHUB_SOURCE}/api" >> /usr/local/bin/compile.sh && \
    echo "go build -gcflags \"all=-N -l\" -buildvcs=false -o /usr/local/bin/dbhub-api ." >> /usr/local/bin/compile.sh && \
    echo "cd ${DBHUB_SOURCE}/db4s" >> /usr/local/bin/compile.sh && \
    echo "go build -gcflags \"all=-N -l\" -buildvcs=false -o /usr/local/bin/dbhub-db4s ." >> /usr/local/bin/compile.sh && \
    echo "cd ${DBHUB_SOURCE}/live" >> /usr/local/bin/compile.sh && \
    echo "go build -gcflags \"all=-N -l\" -buildvcs=false -o /usr/local/bin/dbhub-live ." >> /usr/local/bin/compile.sh && \
    echo "cd ${DBHUB_SOURCE}/webui" >> /usr/local/bin/compile.sh && \
    echo "go build -gcflags \"all=-N -l\" -buildvcs=false -o /usr/local/bin/dbhub-webui ." >> /usr/local/bin/compile.sh && \
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
