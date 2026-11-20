# BNPL — Hamravesh Darkoob

Independent BNPL deployment for namespace `parstis-technology-dpay-dev`.

## Files

| File | Role |
| --- | --- |
| `compose.yaml` | Always-on `bnpl` app (Darkoob Compose import) |
| `jobs.yaml` | One-shot `bnpl-migrate` / `bnpl-init` — ENV only for `configs/dbadmin.yaml` |
| `secrets.example` | Panel checklist for secret ENV only (not an `env_file`) |
| `Makefile` | Build/push runtime + dbadmin images to `registry.hamdocker.ir` |

Canonical BNPL runtime ENV: repo-root
[`.env.dev.hamravesh.example`](../../../.env.dev.hamravesh.example)
(keep in sync with `compose.yaml`). `jobs.yaml` only carries placeholders from
[`configs/dbadmin.yaml`](../../../configs/dbadmin.yaml).

## Hamravesh constraints

- Internal DNS: `SVC_NAME.NS.svc` (for example
  `postgres.parstis-technology-dpay-dev.svc`). Do not use Pod IPs.
- Listen on `0.0.0.0` (`GIN_HOST=0.0.0.0`) so cluster traffic reaches the app.
- Darkoob Compose import supports only: `image`, `command`, `entrypoint`,
  `ports`, `environment`, `volumes`, `depends_on` (Compose >= 3.0).
- `jobs.yaml` must use exec-form arrays for dbadmin, e.g.
  `command: ["db", "migrate"]` / `["db", "init-dev"]`. A single string like
  `"db migrate"` becomes one Cobra argv and fails with
  `unknown command "db migrate"`.

## Images

```sh
make image TAG=v0.1.0-development BUILD_ENV=development
make push-to-hamravesh TAG=v0.1.0-development
make image-dbadmin TAG=v0.1.0-development BUILD_ENV=development
make push-dbadmin-to-hamravesh TAG=v0.1.0-development
```

Or from `deploy/`: `make bnpl-image` / `make bnpl-push-to-hamravesh`.
