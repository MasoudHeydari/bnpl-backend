# dpay — Darkoob standalone deployment project

This folder is self-contained and can be moved into a separate repository.
**Each team has its own independent deployment.** Do not merge team Compose
files into a unified manifest. Darkoob resolves service names as
`<service>.parstis-technology-dpay-dev.svc` within the intended environment.

| Deployment | Always-on containers | Separate one-shot jobs |
| --- | --- | --- |
| infrastructure | Postgres, Redis, Kafka (`compose.yaml` service `kafka`), Temporal, Temporal UI, Loki, Promtail, Grafana | Kafka topics (`jobs.yaml` service `kafka-init`; image under `kafka-init/`) |
| gateway | Envoy | — |
| bnpl (`bnpl/hamravesh`) | API and internal scheduled runtime | migrate, disposable dev bootstrap |
| identity | API, outbox, revocation, notification workers | migrate, seed |
| customer | API | migrate |
| financial-core | API, outbox, provider-start, accounting workers | migrate |
| notification | API, dispatch worker, optional outbox | migrate |

Each team's `compose.yaml` is independent; its `jobs.yaml` holds one-shot
operations and should NOT be deployed as a constantly running application.
A one-shot container must exit successfully before its dependent application
is considered initialized. The `secrets.example` files are **only checklists**
for ENV values to set in the Darkoob panel; they are not `env_file` inputs.

## Operator procedure

1. Build/publish infrastructure Postgres, Kafka init, and Gateway Envoy images
   from the included Dockerfiles. Publish team-owned images separately.
2. Configure storage, required credentials and network policy; deploy
   `infrastructure/compose.yaml`. Persistent volume ownership stays with infra.
   The PostgreSQL first-boot script creates five databases (bnpl, identity,
   customer, financial_core, notification) but not their application roles.
3. Run Kafka topic initialization from `infrastructure/jobs.yaml` once after
   Kafka is reachable. The broker itself is the `kafka` service in
   `infrastructure/compose.yaml`; `kafka-init/` is only the one-shot topic
   bootstrap image/script. `KAFKA_TOPICS` is set on the `kafka-init` job in
   `jobs.yaml` (keep aligned with `docker-compose.local.yml` defaults). Its
   script is copied into its own image, not bind-mounted.
4. Provision service DB roles and run the team's `jobs.yaml` migrations. BNPL
   DBAdmin uses the command **`db migrate`**; do NOT use `migrate up`. It
   requires separately provisioned admin, bnpl_migrator, and bnpl logins. On
   an empty *throwaway development* database only, `bnpl-init` uses the
   destructive `db init-dev` command; never run it against valuable data.
5. Deploy team `compose.yaml` files independently, then Gateway.
   Service names must match the Hamravesh namespace `parstis-technology-dpay-dev`.

## Configuration contracts

Internal service DNS follows Hamravesh Darkoob FAQ form
`SVC_NAME.NS.svc` (not Pod/Service IP). For this environment:

- Postgres: `postgres.parstis-technology-dpay-dev.svc:5432`
- Kafka: `kafka.parstis-technology-dpay-dev.svc:29092`
- Redis: `redis.parstis-technology-dpay-dev.svc:6379`
- Temporal: `temporal.parstis-technology-dpay-dev.svc:7233`
- Team API/gRPC URLs follow the same internal DNS pattern. Keep Kubernetes
  network access restricted; this bundle does not create NetworkPolicies.
- Darkoob Compose import supports only: `image`, `command`, `entrypoint`,
  `ports`, `environment`, `volumes`, `depends_on` (Compose >= 3.0). Do not
  rely on `name`, `restart`, `profiles`, `networks`, `env_file`,
  `healthcheck`, `build`, `container_name`, `tmpfs`, or YAML anchors/`x-*`
  merge keys.
  ([docs](https://docs.hamravesh.com/products/darkube/create/docker-compose/)).
- Darkoob disks are per-app (replica must stay 1) and are not shared across
  apps ([disk docs](https://docs.hamravesh.com/products/darkube/manage/disk/)).
  Named Compose volumes become per-app disks after import; do not assume
  Docker-style shared named volumes between `promtail` and `bnpl`.
- BNPL ENV checklist for Darkoob: repo-root `.env.dev.hamravesh.example`
  (mirror of `.env.example` with Hamravesh DNS defaults). Manifests under
  `deploy/bnpl/hamravesh/` stay in sync with that file.
- BNPL: `APP_EXECUTABLE=./bnpl`, `MESSAGING_PROVIDER=kafka`,
  `POSTGRES_DB=bnpl`, `GIN_HOST=0.0.0.0`. BNPL uses PGPass roles, **not** the
  shared Postgres bootstrap user. Placeholder passwords and any `REPLACE_...`
  values must be entered/corrected in Darkoob ENV before deployment.
- Hamravesh/dev BNPL runtime images (`runtime-development`) bake outbound
  gRPC mTLS PEMs from `deploy/bnpl/hamravesh/certs` into
  `/home/bnpluser/app/certs`. Compose points `EXTERNAL_*_GRPC_TLS_*_FILE`
  there. Production images use target `runtime` without client keys.
  Confirm CA, SAN/server name, and TLS settings for FC, Customer, and
  Notification.
- Envoy JWT issuer/audience and `GATEWAY_TRUST_PROOF` are runtime ENV,
  not image literals. **Rotate the gateway proof that was previously publicly
  committed**. TLS cert and key remain in the gitignored Gateway image
  build context (do not commit private keys).
- Financial Core's payment simulator has a team-local state volume. Do not
  scale its provider-start writer without verifying singleton semantics.
- Temporal currently runs `server start-dev` without durable storage; use
  only for disposable development, never persistent financial workflows.
- Some BNPL expiry/reconciliation jobs are still disabled in the existing
  handoff. Review business schedules before expecting production-like
  payment correctness. Merchant/Location services are not bundled here.
- Envoy body-preview fields and BNPL HTTP debug/response-body logging are
  disabled in these deployments to reduce accidental personal-data exposure.

## Local development preservation

The root `Makefile`, `docker-compose.local.yml`, `.env.example`,
`scripts/kafka/init-topics.sh`, `assets/local-development/` and
the existing Go Dockerfile are unchanged. `make up` and `make down` still
use `docker-compose.local.yml`. The old unified
`docker-compose.dev.yml` handoff is replaced by this folder; local assets
are intentionally duplicated where needed. No CI/CD changes.

## Image build / Hamravesh push (Makefiles)

Each deploy app has its own Makefile. **Build and push are separate**
(`push-to-hamravesh` never rebuilds). Image name/tag/registry are Makefile
env vars (`TAG`, `IMAGE_NAME`, `REGISTRY`, `REGISTRY_NAMESPACE`, …).

From `deploy/`:

```sh
make help

# BNPL (built from repo-root Dockerfile)
make bnpl-image
make bnpl-push-to-hamravesh
make bnpl-image-dbadmin
make bnpl-push-dbadmin-to-hamravesh

# Gateway Envoy
make gateway-image
make gateway-push-to-hamravesh

# Infrastructure images owned by this repo
make postgres-image
make postgres-push-to-hamravesh
make kafka-init-image
make kafka-init-push-to-hamravesh
make loki-image
make loki-push-to-hamravesh
make promtail-image
make promtail-push-to-hamravesh
make grafana-image
make grafana-push-to-hamravesh
```

Or run inside an app directory:

```sh
cd deploy/bnpl/hamravesh && make image TAG=v0.1.0-development BUILD_ENV=development
cd deploy/bnpl/hamravesh && make push-to-hamravesh TAG=v0.1.0-development
cd deploy/infrastructure/kafka-init && make image && make push-to-hamravesh
cd deploy/gateway && make image TAG=v0.1.0
```

Identity / Customer / Financial Core / Notification images are **not** built
from this monorepo. Their Makefiles only support `tag` + `push-to-hamravesh`
after you already have a local image (`SOURCE_IMAGE=... make -C identity tag`).
