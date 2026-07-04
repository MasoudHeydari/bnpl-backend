# Local Loki stack configs

Configs for the local Docker Compose Loki / Promtail / Grafana stack.

| Path | Role |
|------|------|
| `local-config.yaml` | Loki single-binary config (filesystem TSDB, 744h retention) |
| `../promtail/config.yml` | Promtail scrape jobs for eight Parstis services |
| `../grafana/provisioning/` | Grafana Loki datasource |
| `../secrets/` | Grafana admin password Docker secret (not committed) |
| `../logs/<service>/` | Default host log dirs mounted read-only into Promtail |
| `scripts/` | Optional verify helpers (`verify-loki.sh`, etc.) |

## Bring up

From the repository root (after `cp .env.example .env`):

```bash
make up
```

That starts the full local stack (DB, Kafka, Temporal, Envoy, Loki, Promtail,
Grafana, …). The Makefile creates the Grafana password secret and empty log
dirs when they are missing.

- Grafana: http://127.0.0.1:3001 (user `admin`, password from the secret file)
- Loki: http://127.0.0.1:3100 (no auth; loopback only)

Point `BNPL_LOG_DIR` (and sibling `*_LOG_DIR` vars) at real host log directories when
the default `assets/local-development/logs/*` folders are empty placeholders.
