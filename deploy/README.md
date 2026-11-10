# dpay — Darkoob standalone deployment project

This folder is self-contained and can be moved into a separate repository.
**Each team has its own independent deployment.** Do not merge team Compose
files into a unified manifest. Darkoob resolves service names as
`<service>.parstis-technology-dpay-dev.svc` within the intended environment.

| Deployment | Always-on containers | Separate one-shot jobs |
| --- | --- | --- |
| infrastructure | Postgres, Redis, Kafka, Temporal, Temporal UI | Kafka topics |
| gateway | Envoy | — |
| bnpl | API and internal scheduled runtime | migrate, disposable dev bootstrap |
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
   Kafka is reachable. Its script is copied into its own image, not bind-mounted.
4. Provision service DB roles and run the team's `jobs.yaml` migrations. BNPL
   DBAdmin uses the command **`db migrate`**; do NOT use `migrate up`. It
   requires separately provisioned admin, bnpl_migrator, and bnpl logins. On
   an empty *throwaway development* database only, `bnpl-init` uses the
   destructive `db init-dev` command; never run it against valuable data.
5. Deploy team `compose.yaml` files independently, then Gateway.
   Service names must match the Hamravesh namespace `parstis-technology-dpay-dev`.

## Configuration contracts

- Postgres: `postgres.parstis-technology-dpay-dev.svc:5432`
- Kafka: `kafka.parstis-technology-dpay-dev.svc:29092`
- Redis: `redis.parstis-technology-dpay-dev.svc:6379`
- Temporal: `temporal.parstis-technology-dpay-dev.svc:7233`
- Team API/gRPC URLs follow the same internal DNS pattern. Keep Kubernetes
  network access restricted; this bundle does not create NetworkPolicies.
- BNPL: `APP_EXECUTABLE=./bnpl`, `MESSAGING_PROVIDER=kafka`,
  `POSTGRES_DB=bnpl`. BNPL uses PGPass roles, **not** the shared Postgres
  bootstrap user. Placeholder passwords and any `REPLACE_...` values must
  be entered/corrected in Darkoob ENV before deployment.
- The published BNPL image does not include mTLS client PEMs. The runtime
  Compose file decodes panel-provided base64 PEMs to private files under
  `/tmp/dpay-grpc` before launching the existing entrypoint. Confirm CA,
  SAN/server name, and TLS setting for FC, Customer and Notification; some
  upstream handoff values in these configs are intentionally unfilled.
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

## Infrastructure image commands

```sh
cd deploy/infrastructure/postgres && make push-to-hamravesh
cd deploy/gateway/envoy && make push-to-hamravesh

docker build -t registry.hamdocker.ir/parstis-technology/dpay-kafka-init:v0.1.0 deploy/infrastructure/kafka-init
docker push registry.hamdocker.ir/parstis-technology/dpay-kafka-init:v0.1.0
```
