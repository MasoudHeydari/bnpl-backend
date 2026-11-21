# Hamravesh / development outbound gRPC mTLS

These PEMs are baked into the **development** BNPL runtime image
(`Dockerfile` target `runtime-development`) at:

```text
/home/bnpluser/app/certs/ca.crt
/home/bnpluser/app/certs/bnpl-service.crt
/home/bnpluser/app/certs/bnpl-service.key
```

PEMs in this directory are gitignored (except this README). Place them locally
before `make image` (BUILD_ENV=development):

| File | Role |
| --- | --- |
| `ca.crt` | Trusted CA for Financial Core / Customer / Notification |
| `bnpl-service.crt` | BNPL client certificate |
| `bnpl-service.key` | BNPL client private key (pairs with the cert) |

`bnpl-service.csr` is build-input only and is **not** copied into the image.

Production images use target `runtime` and must not include these files.
Do not paste PEMs into Darkoob ENV when using the development image.
