# Envoy TLS material (build context)

Place next to this README (gitignored):

- `tls.crt` — certificate chain (SAN: `dev.dlend.ir`, `api.dev.dlend.ir`, `s.dev.dlend.ir`)
- `tls.key` — private key

Copied into `dpay-envoy:v0.1.0` by `../Dockerfile` via `COPY certs/tls.*`.
