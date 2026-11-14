# Envoy TLS material for `dev.dlend.ir`

Place:

- `tls.crt` — certificate chain (SAN: `dev.dlend.ir`, `api.dev.dlend.ir`, `s.dev.dlend.ir`)
- `tls.key` — private key

These files are gitignored. Envoy mounts this directory at `/etc/envoy/certs`.
