# dpay Hamravesh / self-hosted Compose (`dev.dlend.ir`)

Unified stack defined by `docker-compose.dev.yml` (Compose project name **`dpay`**).

Public edge is **our Envoy** on **443 (TLS only)** — not the Hamravesh product gateway. No plaintext :80.

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

All runtime keys are declared inline under each service `environment:` as plain
literals (e.g. `GIN_HOST: 0.0.0.0`) — no `${VAR:-default}` and no quoting.
Edit values in the compose file or in the Hamravesh panel.

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

1. Build/push own images first (Compose has no `build:` / host bind mounts):

   ```bash
   cd assets/dev/postgres && make push-to-hamravesh   # multi-DB init baked in
   cd ../envoy && make push-to-hamravesh              # needs certs/tls.{crt,key}
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

DNS for the host should point `dev.dlend.ir` (and ideally `api.dev.dlend.ir`, `s.dev.dlend.ir`) at the machine/LB that publishes Envoy **443**.

## Notes / limits (from handoff)

- Image names/digests are hardcoded in Compose (Hamravesh does not interpolate `${…}` on `image:`).
- Payment simulator needs the shared volume `dpay_payment_simulator_data` (already mounted on FC roles). Keep a single provider-start writer.
- Location service is **not** in this Compose; Customer paths that need Location stay unverified until it exists.
- Envoy config is the full platform gateway (JWT/Lua/`x-auth-*`/CORS) adapted from
  `assets/local-development/envoy/development/config.yaml` for Compose DNS + TLS-only :443.
  Rebuild/push `dpay-envoy:v0.1.0` after editing `envoy.yaml` or rotating certs.
- Public edge remains Envoy `443:443` (TLS-only). Other services declare single
  `ports:` mappings for Hamravesh (e.g. `5432:5432`, `8080:8080`); keep them off
  the public internet and reach them via the private network when possible.
- Hamravesh rejects port ranges and `IP:port:port` / `${VAR:-default}` in `ports:` —
  use only `"host:container"` singles.
