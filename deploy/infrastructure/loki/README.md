# dpay Loki

Own image: `registry.hamdocker.ir/parstis-technology/dpay-loki:v0.1.0`

Bakes `config.yaml` + `entrypoint.sh` so Hamravesh does not need host bind
mounts. Single-binary filesystem TSDB, 744h retention, no auth (dev only).

```bash
cd deploy/infrastructure/loki
make image
make push-to-hamravesh
```

Internal DNS after deploy: `loki.parstis-technology-dpay-dev.svc:3100`

Promtail pushes to `http://loki.parstis-technology-dpay-dev.svc:3100/loki/api/v1/push`.
Grafana Explore uses the same host.
