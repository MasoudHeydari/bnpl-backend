# BNPL — Hamravesh Darkoob

Independent BNPL deployment for namespace `parstis-technology-dpay-dev`.

## Files

| File | Role |
| --- | --- |
| `compose.yaml` | Always-on `bnpl` app (Darkoob; Alloy logging image + BNPL-local log disk) |
| `logging/` | Edge Alloy + supervisord + logrotate configs baked into `runtime-development-alloy` |
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

### Sentry (error reporting)

Compose sets `ERROR_REPORTING_PROVIDER=sentry`. Put `SENTRY_DSN` (and
optionally `SENTRY_RELEASE`) in the Darkoob panel from `secrets.example`.
The app already reports panics and unexpected HTTP/runtime errors through the
core `errorreporting` port and the Sentry adapter — no `sentrygin` middleware
or process-global `sentry.Init` is required.

## Logging (Edge Alloy)

Hamravesh uses image tag `v0.1.0-development-alloy` (`runtime-development-alloy`):
supervisord runs Edge Alloy + BNPL + logrotate. Logs ship over HTTP to Central
Alloy (`ALLOY_PUSH_URL`). See [`logging/README.md`](logging/README.md).

```sh
make image-alloy
make push-alloy-to-hamravesh
```

Keep `LOG_PRETTY_PRINT=false` so Alloy tails compact JSONL (`LOG_LEVEL` may be
`debug`).

## Hamravesh constraints

- Internal DNS: `SVC_NAME.NS.svc` (for example
  `postgres.parstis-technology-dpay-dev.svc`). Do not use Pod IPs.
- Listen on `0.0.0.0` (`GIN_HOST=0.0.0.0`) so cluster traffic reaches the app.
- Darkoob Compose import supports only: `image`, `command`, `entrypoint`,
  `ports`, `environment`, `volumes`, `depends_on` (Compose >= 3.0).
  See [Hamravesh docs](https://docs.hamravesh.com/products/darkube/create/docker-compose/).
- Darkoob disks are per-app. Do not rely on shared volumes between `bnpl` and
  collectors; Edge Alloy pushes to Central Alloy instead.
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
make image-alloy
make push-alloy-to-hamravesh
make image-dbadmin TAG=v0.1.0-development BUILD_ENV=development
make push-dbadmin-to-hamravesh TAG=v0.1.0-development
```

Or from `deploy/`: `make bnpl-image` / `make bnpl-image-alloy` /
`make bnpl-push-to-hamravesh` / `make bnpl-push-alloy-to-hamravesh`.
