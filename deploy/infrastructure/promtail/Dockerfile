# dpay Promtail — config + entrypoint baked in (Hamravesh rejects host bind mounts).
#
#   docker build -t registry.hamdocker.ir/parstis-technology/dpay-promtail:v0.1.0 \
#     -f deploy/infrastructure/promtail/Dockerfile deploy/infrastructure/promtail
FROM docker.arvancloud.ir/grafana/promtail:3.6.11

USER root

RUN groupadd --gid 10001 promtail \
    && useradd \
        --uid 10001 \
        --gid promtail \
        --no-create-home \
        --shell /usr/sbin/nologin \
        promtail \
    && mkdir -p /var/lib/promtail /var/log/services \
    && chown -R promtail:promtail \
        /var/lib/promtail \
        /var/log/services

COPY --chmod=644 config.yml /etc/promtail/config.yml
COPY --chmod=755 entrypoint.sh /entrypoint.sh

USER promtail:promtail

ENTRYPOINT ["/entrypoint.sh"]
