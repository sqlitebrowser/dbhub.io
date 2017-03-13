Hah, you might think! Got their secret crypto just by doing a simple search
on Github, Google, or whatever! But don't start celebrating just yet. There
is nothing interesting to see here. These certificates and keys are not used
in any publicly available application. They are for local testing purposes
only. Have fun using them in your application as well :)

They have been generated using the in-depth guide here:

* https://jamielinux.com/docs/openssl-certificate-authority/

The .crt files can be viewed using OpenSSL:

    $ openssl x509 -noout -text -in [filename]

For eample:

```
$ openssl x509 -noout -text -in intermediate.crt
Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number: 4096 (0x1000)
        Signature Algorithm: sha256WithRSAEncryption
        Issuer: C=GB, ST=England, O=Alice Ltd, OU=Alice Ltd Certificate Authority, CN=Alice Ltd Root CA
        Validity
            Not Before: Oct 24 16:28:15 2016 GMT
            Not After : Oct 22 16:28:15 2026 GMT
        Subject: C=GB, ST=England, O=Alice Ltd, CN=Alice Ltd Intermediate CA
        Subject Public Key Info:
            Public Key Algorithm: rsaEncryption
            RSA Public Key: (4096 bit)
                Modulus (4096 bit):
                    00:b9:33:c7:e8:8c:fd:fb:17:b5:eb:29:99:60:b6:
                    45:79:9e:c7:5b:13:6b:d9:34:ac:d0:22:2c:ab:9c:
                    da:52:8d:fa:ec:1a:eb:27:2d:9b:9e:6a:44:fe:5e:
                    48:38:67:e1:41:78:95:b7:9e:15:3f:8a:57:d3:0d:
                    e6:b6:7e:3b:52:f7:3c:c6:08:6c:6d:7c:96:f9:92:
                    8e:6c:a1:80:67:ef:ec:bf:4c:6f:44:95:17:91:2f:
                    5a:02:b5:90:2a:ac:ac:75:35:f1:21:b6:f9:c7:09:
                    c4:5f:cb:de:98:5e:ad:5f:7a:7b:dd:99:9e:1c:74:
                    5e:36:a1:b5:8d:0f:d6:0e:6b:c5:ba:06:56:fb:f1:
                    20:aa:0b:9b:eb:5b:d6:b2:61:32:27:f0:cf:39:27:
                    57:81:b3:70:48:f1:80:c8:07:1d:04:2a:b3:5e:bd:
                    55:de:7e:ec:80:08:fd:0f:80:b3:bd:57:f8:42:6e:
                    0f:30:51:01:50:2f:f8:ad:55:1b:a3:5c:8c:67:6a:
                    26:2e:a1:22:1f:b0:f5:2a:1d:8f:16:d3:3d:cf:ae:
                    08:24:bc:01:bc:cc:14:b1:5c:19:65:07:8b:b6:1b:
                    bc:2d:9b:3b:b9:57:40:d1:c8:db:14:f1:1d:0b:c5:
                    d7:7f:66:fc:3a:ef:30:b6:b0:cd:b5:5e:ef:3e:b8:
                    f6:32:ef:f9:c9:f2:bf:d1:ac:b9:17:20:a9:36:b0:
                    f7:01:52:c7:d2:57:bc:b8:92:ca:78:08:60:23:de:
                    70:58:6f:24:25:9b:b8:50:3e:51:38:70:d8:df:93:
                    2f:3f:7a:8b:1a:e7:3b:83:bf:f8:f5:72:8c:ba:0b:
                    5e:c0:cf:ab:cc:a0:4d:62:a2:5f:df:6f:16:21:a5:
                    d1:61:d8:25:53:9c:02:19:40:b2:34:05:e4:51:8a:
                    d9:12:a4:d7:d6:08:07:cb:3c:68:0e:5a:02:bf:1c:
                    58:52:a5:c8:9d:47:07:c8:60:0e:09:39:8f:14:8d:
                    b8:a2:23:c8:68:bb:d2:d9:2a:c9:77:f2:71:e3:41:
                    60:ae:8a:d6:ed:b7:21:25:1d:5f:8e:1b:61:59:b6:
                    6d:88:58:bd:10:bf:28:3c:cf:d9:ea:85:91:af:11:
                    63:4a:87:79:15:aa:d7:80:0a:a4:80:6c:0e:42:0c:
                    1e:03:19:93:2e:ad:e4:d8:2c:10:25:ca:f2:5d:a7:
                    13:cf:de:39:19:c0:a1:de:29:62:b5:1f:9a:87:6b:
                    0e:17:ec:7e:85:66:38:08:be:a4:a8:fe:99:73:7d:
                    e7:0f:6c:a3:ca:41:cd:70:74:0a:46:a4:fe:ee:f1:
                    fa:0f:23:11:7d:98:36:21:8b:49:6d:ca:ea:00:a4:
                    04:ed:23
                Exponent: 65537 (0x10001)
        X509v3 extensions:
            X509v3 Subject Key Identifier: 
                53:36:5D:B5:46:18:86:BA:2B:3F:C5:31:D7:87:DB:67:15:C5:2D:68
            X509v3 Authority Key Identifier: 
                keyid:8C:84:0A:13:88:79:01:56:3E:ED:C8:F0:E0:F4:BC:EA:89:0C:99:11

            X509v3 Basic Constraints: critical
                CA:TRUE, pathlen:0
            X509v3 Key Usage: critical
                Digital Signature, Certificate Sign, CRL Sign
    Signature Algorithm: sha256WithRSAEncryption
        11:d1:c3:b5:fd:db:22:67:4c:26:f1:ef:25:9c:9f:d5:43:30:
        d6:fc:ce:e2:fb:b3:93:66:76:a6:2f:ba:32:7c:45:fb:01:6d:
        f1:9c:81:fa:aa:aa:a5:e6:3b:ff:42:db:e6:05:90:6c:bf:cd:
        25:e4:b4:7e:71:eb:e5:05:70:ac:cc:72:aa:32:26:fd:4c:21:
        ae:15:52:1d:59:45:3a:83:ea:98:96:88:e5:96:10:99:91:21:
        19:d6:28:d8:17:82:40:87:26:ff:07:f0:f6:89:2e:ae:85:1c:
        b9:65:69:f2:01:8f:7c:d1:93:d4:ec:2a:d9:8b:01:fb:ca:87:
        50:7c:0d:d0:71:5d:c1:f9:75:a7:b7:48:1a:98:88:3b:bc:f0:
        97:b2:bd:f4:d2:f8:14:6d:39:b7:86:ae:1b:f8:0c:ed:ea:f8:
        d8:68:fd:aa:bd:d7:29:29:f2:ea:ba:ae:0e:b0:6f:29:20:52:
        80:ea:62:c5:ce:ea:ac:95:ef:fa:db:b3:e5:e6:6e:4a:fa:7f:
        6a:0a:e3:c2:15:39:bf:f9:96:d5:3e:22:5d:ea:3f:7b:6f:ba:
        4d:1a:6e:ea:d8:69:a7:f4:ce:1a:8d:a9:75:dc:e6:9f:03:f9:
        38:42:f0:e9:d2:d7:c0:07:73:d5:72:27:16:94:96:34:9d:8c:
        d3:18:b7:64:78:bb:17:ed:90:7b:99:be:f2:98:26:a5:14:89:
        79:cf:8a:42:fc:6b:b4:d9:22:eb:b7:70:2e:16:a0:db:f4:48:
        c1:8d:dd:da:d2:f1:09:d9:51:89:2e:b8:9d:fc:8f:74:d6:5e:
        c0:30:f0:56:ec:c9:42:42:a6:54:c8:34:1e:10:4b:f5:2d:ce:
        8d:40:4e:8d:fe:45:80:da:81:63:cf:9b:59:80:4a:c3:56:b1:
        67:55:f7:77:77:e0:32:70:fe:b5:a6:3a:f1:f4:d2:b9:e2:11:
        9d:17:ad:94:3c:97:9d:c3:ac:7f:3a:c1:8b:60:53:2e:eb:15:
        99:36:06:ba:93:86:c4:f6:b4:41:ad:74:fb:70:74:60:47:fc:
        4b:ee:03:ca:7a:df:20:8d:02:00:c5:ab:0e:55:b0:8e:b3:56:
        e4:ce:09:ef:e2:8c:75:2e:b5:32:5e:67:ae:f8:4e:7d:4a:6c:
        e1:1a:32:d5:c1:11:e6:a0:ba:27:a2:25:ca:ac:08:bb:84:f0:
        0c:43:77:87:19:c2:cd:39:31:0b:d4:1e:d8:07:fa:dc:26:28:
        47:d4:e3:09:1d:41:3b:35:ed:db:ec:b8:0d:84:f7:d4:0d:e6:
        d4:b6:b0:91:81:10:8b:60:bb:f8:0c:b8:91:f7:4a:8c:a5:09:
        d9:da:e0:30:d6:2e:a6:af
```
