# dpay Loki — config + entrypoint baked in (Hamravesh rejects host bind mounts).
#
#   docker build -t registry.hamdocker.ir/parstis-technology/dpay-loki:v0.1.0 \
#     -f deploy/infrastructure/loki/Dockerfile deploy/infrastructure/loki
FROM docker.arvancloud.ir/grafana/loki:3.6.8

USER root

COPY config.yaml /etc/loki/config.yaml
COPY --chmod=755 entrypoint.sh /entrypoint.sh

RUN mkdir -p \
		/loki/chunks \
		/loki/rules \
		/loki/compactor \
		/loki/tsdb-index \
		/loki/tsdb-cache \
	&& chown -R 10001:10001 /loki /etc/loki /entrypoint.sh

USER 10001

ENTRYPOINT ["/entrypoint.sh"]
