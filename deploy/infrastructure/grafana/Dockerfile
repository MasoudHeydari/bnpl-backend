# dpay Grafana — Loki datasource provisioning baked in.
#
#   docker build -t registry.hamdocker.ir/parstis-technology/dpay-grafana:v0.1.0 \
#     -f deploy/infrastructure/grafana/Dockerfile deploy/infrastructure/grafana
FROM docker.arvancloud.ir/grafana/grafana:12.3.0

FROM docker.arvancloud.ir/grafana/grafana:13.3.2

COPY --chown=root:root --chmod=755 provisioning /etc/grafana/provisioning
COPY --chown=root:root --chmod=755 entrypoint.sh /entrypoint.sh

USER grafana

ENTRYPOINT ["/entrypoint.sh"]
