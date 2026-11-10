# dpay Postgres

Own image: `registry.hamdocker.ir/parstis-technology/dpay-postgres:v0.1.0`

Bakes `init-databases.sh` into `/docker-entrypoint-initdb.d/01-init-databases.sh`
so first boot creates empty DBs `identity`, `customer`, `financial_core`, and
`notification` alongside `POSTGRES_DB` (`bnpl`). No host bind mount (Hamravesh).

```bash
cd deploy/infrastructure/postgres
make image
make push-to-hamravesh
```

Init scripts run only when the data volume is empty (first Postgres boot).

The init script only creates databases, not individual application roles.
