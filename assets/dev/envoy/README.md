# dpay Envoy gateway

TLS-only edge on **443** (no plaintext HTTP :80), including development.

Own image: `registry.hamdocker.ir/parstis-technology/dpay-envoy:v0.1.0`

`envoy.yaml` and TLS material are **baked into the image** at build time (Hamravesh
rejects host bind mounts; we do not inject config/certs via ENV base64).

`envoy.yaml` is the full platform gateway snapshot adapted from
`assets/local-development/envoy/development/config.yaml` for Compose DNS + hosts
`dev.dlend.ir` / `api.dev.dlend.ir` / `s.dev.dlend.ir`.

| Item | Value |
| --- | --- |
| Listen | `0.0.0.0:443` |
| Short-link host | `s.dev.dlend.ir` |
| Upstreams | `dpay-bnpl:8080`, `dpay-identity:8080`, `dpay-customer:8081`, `dpay-financial-core:8082`, `dpay-merchant:8085` |
| In-image TLS | `/etc/envoy/certs/tls.crt`, `/etc/envoy/certs/tls.key` |

`dpay-merchant` is not in `docker-compose.dev.yml` yet — Merchant routes stay
unreachable until that service joins the network.

JWT `issuer` / `audiences` in the baked config must match Identity
(`IDENTITY_ACCESS_JWT_*`). Rebuild the image when those change.

## Layout (build context = this directory)

```text
assets/dev/envoy/
  Dockerfile
  envoy.yaml
  certs/tls.crt   # gitignored
  certs/tls.key   # gitignored
```

## Build / push

Place `certs/tls.crt` and `certs/tls.key` first (SAN: `dev.dlend.ir`, api/s aliases), then:

```bash
cd assets/dev/envoy
make image                         # local dpay-envoy:v0.1.0 + hamravesh tag
make image TAG=v0.1.1
make push                          # docker push local tag
make push-to-hamravesh             # docker push registry.hamdocker.ir/.../dpay-envoy:v0.1.0
make push-to-hamravesh TAG=v0.1.1
```

Compose only pulls `registry.hamdocker.ir/parstis-technology/dpay-envoy:$(TAG)` —
run `make push-to-hamravesh` before `up` on Hamravesh.

## Validate

```bash
envoy --mode validate -c assets/dev/envoy/envoy.yaml
```
