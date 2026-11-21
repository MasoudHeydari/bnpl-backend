# BNPL — Hamravesh Darkoob

Independent BNPL deployment for namespace `parstis-technology-dpay-dev`.

## Files

| File | Role |
| --- | --- |
| `compose.yaml` | Always-on `bnpl` app (Darkoob Compose import; mounts `dpay_service_logs` for Loki) |
| `jobs.yaml` | One-shot `bnpl-migrate` / `bnpl-init` / `bnpl-import-trusted-customers` |
| `trusted-customers.csv` | Trusted-cohort mobiles baked into the runtime image for the import job |
| `certs/` | Dev outbound gRPC mTLS PEMs baked into `runtime-development` |
| `secrets.example` | Panel checklist for secret ENV only (not an `env_file`) |
| `Makefile` | Build/push runtime + dbadmin images to `registry.hamdocker.ir` |

Canonical BNPL runtime ENV: repo-root
[`.env.dev.hamravesh.example`](../../../.env.dev.hamravesh.example)
(keep in sync with `compose.yaml`). Migrate/init jobs carry placeholders from
[`configs/dbadmin.yaml`](../../../configs/dbadmin.yaml). The trusted-customer
import job uses the runtime image and the same runtime ENV shape as `compose.yaml`
`bnpl` (TLS paths point at image-baked certs under `/home/bnpluser/app/certs/`).

## Hamravesh constraints

- Internal DNS: `SVC_NAME.NS.svc` (for example
  `postgres.parstis-technology-dpay-dev.svc`). Do not use Pod IPs.
- Listen on `0.0.0.0` (`GIN_HOST=0.0.0.0`) so cluster traffic reaches the app.
- Darkoob Compose import supports only: `image`, `command`, `entrypoint`,
  `ports`, `environment`, `volumes`, `depends_on` (Compose >= 3.0).
- `jobs.yaml` must use **flow-form** exec arrays on one line, e.g.
  `command: ["db", "migrate"]` /
  `["policy-assignment", "import-trusted", "--file", "...", "--actor", "..."]`.
  A single string or a block-style YAML list becomes one Cobra argv and fails
  with `unknown command "policy-assignment import-trusted ..."`.
- Development runtime images bake `certs/` into
  `/home/bnpluser/app/certs/{ca,bnpl-service}.{crt,key}` and bake
  `trusted-customers.csv` into
  `assets/csv/trusted-customers/trusted-customers.csv`. Do not set
  `BNPL_GRPC_*_PEM_BASE64` in Darkoob for this flow.
- Run `bnpl-import-trusted-customers` once after Customer Reference and BNPL DB
  are ready. Edit `trusted-customers.csv`, rebuild/push the runtime image, then
  deploy the one-shot job. Import is additive/idempotent.

## Images

```sh
# Requires deploy/bnpl/hamravesh/certs/{ca.crt,bnpl-service.crt,bnpl-service.key}
make image TAG=v0.1.0-development BUILD_ENV=development
make push-to-hamravesh TAG=v0.1.0-development
make image-dbadmin TAG=v0.1.0-development BUILD_ENV=development
make push-dbadmin-to-hamravesh TAG=v0.1.0-development
```

Or from `deploy/`: `make bnpl-image` / `make bnpl-push-to-hamravesh`.
