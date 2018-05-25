This directory contains useful extra pieces used with DBHub.io.

At the moment, it's just the systemd files used for deploying
on CentOS 7.x.

### Notes

The dbhub-webui service binds to port 443.

* On Linux, the executable needs to have the capability
  "CAP_NET_BIND_SERVICE" applied to it, otherwise it won't be
  able to start.

  The dbhub-webui.service file takes care of this for systemd
  (tested on CentOS 7).  If you're not using systemd, then the
  capability can be applied manually prior to starting the
  daemon:

      $ sudo setcap CAP_NET_BIND_SERVICE=ep /usr/local/bin/webui

* On FreeBSD, this can be done by changing the reserved port range
  to exclude 443:

      $ sudo sysctl net.inet.ip.portrange.reservedhigh=442

  Remember to set that in /etc/sysctl.conf too, so the change is
  automatically applied at boot.

