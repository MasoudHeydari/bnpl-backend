# Local Envoy listener TLS materials

Place the development listener certificate and private key here (never commit
the PEM files). Compose mounts this directory at `/etc/envoy/certs`.

```text
README.md
api.bnpl.test.pem
api.bnpl.test-key.pem
```

Copy from `platform-infra/envoy/certs` (or regenerate with mkcert) before
starting the local Envoy container. See
[`doc/local-development/mkcert/README.md`](../../../../../doc/local-development/mkcert/README.md).
