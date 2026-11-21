# dpay Promtail

Own image: `registry.hamdocker.ir/parstis-technology/dpay-promtail:v0.1.0`

Bakes `config.yml` + `entrypoint.sh`. Scrapes
`/var/log/services/<service>/*.{log,json,jsonl}` from the shared
`dpay_service_logs` volume and pushes to Loki.

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

BNPL must mount the same volume at `/var/log/services/bnpl-service` and set
`LOG_FILE=/var/log/services/bnpl-service/bnpl.jsonl` so Promtail can tail it.
Docker socket scraping is intentionally omitted (unsupported on Hamravesh).
