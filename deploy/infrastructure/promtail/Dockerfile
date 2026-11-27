# dpay Promtail — config + entrypoint baked in (Hamravesh rejects host bind mounts).
#
#   docker build -t registry.hamdocker.ir/parstis-technology/dpay-promtail:v0.1.0 \
#     -f deploy/infrastructure/promtail/Dockerfile deploy/infrastructure/promtail
FROM docker.arvancloud.ir/grafana/promtail:3.6.8

USER root

COPY config.yml /etc/promtail/config.yml
COPY --chmod=755 entrypoint.sh /entrypoint.sh

RUN mkdir -p /var/lib/promtail /var/log/services \
	&& chown -R promtail:promtail /var/lib/promtail /var/log/services /etc/promtail /entrypoint.sh

USER promtail

ENTRYPOINT ["/entrypoint.sh"]
