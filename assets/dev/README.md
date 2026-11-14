# dpay Hamravesh / self-hosted Compose (`dev.dlend.ir`)

Unified stack defined by `docker-compose.dev.yml` (Compose project name **`dpay`**).

Public edge is **our Envoy** on ports **80/443** — not the Hamravesh product gateway.

## What runs

| Layer | Containers |
| --- | --- |
| Infra | `dpay-postgres`, `dpay-redis`, `dpay-kafka`, `dpay-temporal`, `dpay-temporal-ui` |
| Gateway | `dpay-envoy` |
| Platform | Identity / Customer / Financial Core / Notification (+ workers) |
| BNPL | `dpay-bnpl` (+ `dpay-bnpl-migrate` profile) |

Postgres creates five DBs on first boot: `bnpl`, `identity`, `customer`, `financial_core`, `notification`.

## ENV model (Hamravesh)

Hamravesh does **not** accept host `env_file` paths.

All runtime keys are declared inline under each service `environment:` as `${VAR}` /
`${VAR:-default}`. Set values in the Hamravesh panel (or a local Compose `.env` on a VM).

Examples:

| Key | Typical value |
| --- | --- |
| `CONFIG_FILE` | `configs/runtime.yaml` (BNPL); dbadmin migrate uses `configs/dbadmin.yaml` |
| `POSTGRES_PASSWORD` | shared Postgres password |
| `GATEWAY_TRUST_PROOF` | shared gateway trust proof |
| `IDENTITY_DB_JDBC_URL` | default `jdbc:postgresql://dpay-postgres:5432/identity` |
| `KAFKA_BROKER_1` / `*_KAFKA_BOOTSTRAP_SERVERS` | default `dpay-kafka:29092` |

Use the handoff `*.env.example.json` as the checklist of required secret keys
(`REPLACE_*`). Connectivity defaults for this Compose network are already baked into
`docker-compose.dev.yml`.

Optional Compose-level vars: copy `.env.dev.example` → `.env` (or export).

## One-time prep

1. TLS for Envoy (SAN must include `dev.dlend.ir` / `api.dev.dlend.ir` / `s.dev.dlend.ir`):

   ```bash
   # place:
   #   assets/dev/certs/tls.crt
   #   assets/dev/certs/tls.key
   ```

2. In Hamravesh (or local `.env`), fill secrets from the handoff JSON templates:
   JWT keys, mTLS `*_FILE_BASE64`, DB passwords, `GATEWAY_TRUST_PROOF`, Kavenegar, etc.

## Boot order

```bash
# 1) Infrastructure + topic init
docker compose -f docker-compose.dev.yml up -d postgres redis kafka temporal
docker compose -f docker-compose.dev.yml up kafka-init

# 2) Finite migrations (do not leave these as always-on)
docker compose -f docker-compose.dev.yml --profile migration run --rm identity-migrate
docker compose -f docker-compose.dev.yml --profile migration run --rm customer-migrate
docker compose -f docker-compose.dev.yml --profile migration run --rm notification-migrate
docker compose -f docker-compose.dev.yml --profile migration run --rm financial-core-migrate
docker compose -f docker-compose.dev.yml --profile migration run --rm bnpl-migrate

# 3) Optional identity seed (once)
docker compose -f docker-compose.dev.yml --profile seed run --rm identity-seed

# 4) Long-running apps + gateway
docker compose -f docker-compose.dev.yml up -d \
  identity identity-outbox identity-revocation identity-notifications \
  customer notification notification-dispatch \
  financial-core financial-outbox financial-provider-start financial-accounting \
  bnpl temporal-ui envoy
```

DNS for the host should point `dev.dlend.ir` (and ideally `api.dev.dlend.ir`, `s.dev.dlend.ir`) at the machine/LB that publishes Envoy 80/443.

## Notes / limits (from handoff)

- Image digests for platform services are pinned in Compose; override with `*_IMAGE` if needed.
- Payment simulator needs the shared volume `dpay_payment_simulator_data` (already mounted on FC roles). Keep a single provider-start writer.
- Location service is **not** in this Compose; Customer paths that need Location stay unverified until it exists.
- Envoy config under `assets/dev/envoy/envoy.yaml` is a routing skeleton (path prefixes). Platform JWT/Lua filters from local Envoy are not fully reproduced — harden before real traffic.
- Do not open Postgres/Redis/Kafka/gRPC publicly; only Envoy 80/443 should face the internet.
