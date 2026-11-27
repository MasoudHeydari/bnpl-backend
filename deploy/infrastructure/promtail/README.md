# dpay Promtail

Own image: `registry.hamdocker.ir/parstis-technology/dpay-promtail:v0.1.0`

Bakes `config.yml` + `entrypoint.sh`. Scrapes
`/var/log/services/<service>/*.{log,json,jsonl}` from the disk mounted at
`/var/log/services` and pushes to Loki.

```bash
cd deploy/infrastructure/promtail
make image
make push-to-hamravesh
```

ENV (optional):

| Key | Default |
| --- | --- |
| `LOKI_PUSH_URL` | `http://loki.parstis-technology-dpay-dev.svc:3100/loki/api/v1/push` |
| `APPLICATION_ENV` | `development` |

## Log path contract

BNPL mounts its disk at `/var/log/services` and sets
`LOG_FILE=/var/log/services/bnpl-service/bnpl.jsonl` so files land under
`/var/log/services/bnpl-service/…`. Promtail scrapes that tree.

Docker socket scraping is intentionally omitted (unsupported on Hamravesh).

## Darkoob disk caveat (important)

Darkoob turns each Compose service into a **separate app** and each named
volume into a **per-app disk**. Disks are not shared across apps or replicas
([disk docs](https://docs.hamravesh.com/products/darkube/manage/disk/)).

The Compose name `dpay_service_logs` is therefore **not** an automatically
shared filesystem between the `promtail` and `bnpl` apps. File-based scrape
only works if both apps actually see the same files (for example a platform-
level shared volume arranged with Hamravesh support, or a dedicated cluster
PVC). Otherwise use another ship path (app → Loki push, sidecar, etc.).

Compose import keys only: `image`, `command`, `entrypoint`, `ports`,
`environment`, `volumes`, `depends_on`
([docs](https://docs.hamravesh.com/products/darkube/create/docker-compose/)).
