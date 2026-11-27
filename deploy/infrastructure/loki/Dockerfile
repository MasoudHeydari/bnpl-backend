# Loki — config + entrypoint baked in (Hamravesh rejects host bind mounts).
# Compatible with Hamravesh Darkoob.
#
#   docker build -t registry.hamdocker.ir/parstis-technology/dpay-loki:v0.1.0 \
#     -f deploy/infrastructure/loki/Dockerfile deploy/infrastructure/loki

FROM docker.arvancloud.ir/grafana/loki:3.7.10

COPY --chown=loki:loki config.yaml /etc/loki/config.yaml
COPY --chmod=755 entrypoint.sh /entrypoint.sh

USER loki

EXPOSE 3100

ENTRYPOINT ["/entrypoint.sh"]
