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

## Darkoob notes

- Compose import keys only: `image`, `command`, `entrypoint`, `ports`,
  `environment`, `volumes`, `depends_on`
  ([docs](https://docs.hamravesh.com/products/darkube/create/docker-compose/)).
- Persist `/loki` via a Darkoob disk (Compose volume `dpay_loki_data`). Disk
  apps must keep replica = 1
  ([disk docs](https://docs.hamravesh.com/products/darkube/manage/disk/)).
- Image runs as user `loki`; the entrypoint creates chunk/index dirs under
  `/loki` before starting the binary.
