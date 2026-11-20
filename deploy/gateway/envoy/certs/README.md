# Envoy TLS material (build context)

Place next to this README (gitignored):

- `tls.crt` — certificate chain (SAN: `dev.dlend.ir`, `api.dev.dlend.ir`; the legacy `s.dev.dlend.ir` SAN is no longer required because short links resolve on the public site host as `dev.dlend.ir/s/<code>`)
- `tls.key` — private key

Copied into `dpay-envoy:v0.1.0` by `../Dockerfile` via `COPY certs/tls.*`.
