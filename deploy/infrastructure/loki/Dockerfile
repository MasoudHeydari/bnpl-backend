# Loki — config + entrypoint baked in (Hamravesh rejects host bind mounts).
# Compatible with Hamravesh Darkoob.
#
# Upstream grafana/loki is distroless (no /bin/sh). A shell entrypoint cannot
# run there, so we copy the statically linked binary onto Alpine.
#
#   docker build -t registry.hamdocker.ir/parstis-technology/dpay-loki:v0.1.0 \
#     -f deploy/infrastructure/loki/Dockerfile deploy/infrastructure/loki

FROM docker.arvancloud.ir/grafana/loki:3.7.10 AS loki

FROM docker.arvancloud.ir/alpine:3.23

RUN apk add --no-cache ca-certificates \
	&& addgroup -g 10001 -S loki \
	&& adduser -u 10001 -S -G loki -H -s /sbin/nologin loki \
	&& mkdir -p /loki /etc/loki \
	&& chown -R loki:loki /loki /etc/loki

COPY --from=loki /usr/bin/loki /usr/bin/loki
COPY --chown=loki:loki config.yaml /etc/loki/config.yaml
COPY --chmod=755 entrypoint.sh /entrypoint.sh

USER loki

EXPOSE 3100

ENTRYPOINT ["/entrypoint.sh"]
