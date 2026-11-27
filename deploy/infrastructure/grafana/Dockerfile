# dpay Grafana — Loki datasource provisioning baked in.
#
#   docker build -t registry.hamdocker.ir/parstis-technology/dpay-grafana:v0.1.0 \
#     -f deploy/infrastructure/grafana/Dockerfile deploy/infrastructure/grafana
FROM docker.arvancloud.ir/grafana/grafana:12.3.0

USER root

COPY provisioning /etc/grafana/provisioning
COPY --chmod=755 entrypoint.sh /entrypoint.sh

RUN chown -R grafana:root /etc/grafana/provisioning /entrypoint.sh

USER grafana

ENTRYPOINT ["/entrypoint.sh"]
