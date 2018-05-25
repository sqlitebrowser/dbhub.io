### systemd unit files used in our (current) deployment on Scaleway

* dbhub-db4s.service

  Starts the DBHub.io DB4S end point listener on port 5550.

  This is what DB Browser for SQLite (DB4S) connects to for remote
  database operations.


* dbhub-webui.service

  Starts the DBHub.io web service on port 443 (HTTPS).


* mnt-minio.mount

  Mounts a secondary block device under `/mnt/minio` at boot.


* minio.service

  Starts the Minio service.  This is a customised version of the Minio
  service file found [here](https://github.com/minio/minio-service/tree/master/linux-systemd).

  The customisation is simply to add a dependency on the above
  mnt-minio.mount.
