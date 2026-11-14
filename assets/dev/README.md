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

## One-time prep

1. Copy handoff env JSON → private env files (never commit):

   ```bash
   mkdir -p private
   # From parstis-devops-handoff-20261112:
   # fill identity/customer/financial-core/notification env JSON, then:
   python3 /path/to/handoff/prepare-env.py
   # Ensure these exist (Compose requires them):
   #   private/identity.env
   #   private/customer.env
   #   private/financial-core.env
   #   private/notification.env
   #   private/bnpl.env
   ```

2. Inside those private env files, point dependencies at Compose DNS, for example:

   - JDBC / DB host: `dpay-postgres`
   - Kafka bootstrap: `dpay-kafka:29092`
   - Redis: `redis://dpay-redis:6379`
   - Identity gRPC host (from Customer/FC): `dpay-identity`
   - Customer gRPC host (from FC): `dpay-customer`

3. TLS for Envoy (SAN must include `dev.dlend.ir` / `api.dev.dlend.ir` / `s.dev.dlend.ir`):

   ```bash
   # place:
   #   assets/dev/certs/tls.crt
   #   assets/dev/certs/tls.key
   ```

4. Optional Compose-level vars: copy `.env.dev.example` → `.env` (or export).

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
