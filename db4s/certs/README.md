## Certificates to run a DB4S end point server

These are just development certificates, not what is used in
production.  There is no password on server.key, so you should
be able to use these "as is" by updating your ~/.dbhub/config.toml
to point at them.

    [db4s]
    server = "db4s-dev.dbhub.io"
    port = 5550
    certificate = "/path/to/server.crt"
    certificate_key = "/path/to/server.key"
    ca_chain = "/path/to/ca-chain.crt"

You'll need to make sure your development DB4S end point server
is found in DNS as "db4s-dev.dbhub.io" though, by adding it to
your /etc/hosts or similar.


## Useful commands

To view the details of a certificate:

    $ openssl x509 -noout -text -in somecert.crt


## Useful certificate reference info

  https://jamielinux.com/docs/openssl-certificate-authority/
