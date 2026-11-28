# dpay Grafana

Own image: `registry.hamdocker.ir/parstis-technology/dpay-grafana:v0.1.0`

Bakes Loki datasource provisioning
(`http://loki.parstis-technology-dpay-dev.svc:3100`) and a thin entrypoint.

```bash
cd deploy/infrastructure/grafana
make image
make push-to-hamravesh
```

Set in Darkoob panel (required):

| Key | Notes |
| --- | --- |
| `GF_SECURITY_ADMIN_PASSWORD` | Admin password (do not commit). Entrypoint exits if unset. |
| `GF_SERVER_ROOT_URL` | Public URL if exposed via gateway, e.g. `https://grafana.dev.example` |

Default admin user: `admin` (`GF_SECURITY_ADMIN_USER`).

UI port inside the container is `3000` (compose publishes it).

## Darkoob notes

- Compose import keys only: `image`, `command`, `entrypoint`, `ports`,
  `environment`, `volumes`, `depends_on`
  ([docs](https://docs.hamravesh.com/products/darkube/create/docker-compose/)).
- Persist `/var/lib/grafana` via a Darkoob disk (Compose volume
  `dpay_grafana_data`); keep replica = 1
  ([disk docs](https://docs.hamravesh.com/products/darkube/manage/disk/)).
- Image runs as user `grafana`.
