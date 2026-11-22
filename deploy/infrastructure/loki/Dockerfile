# Loki — config + entrypoint baked in (Hamravesh rejects host bind mounts).
# Compatible with Hamravesh Darkoob.

FROM docker.arvancloud.ir/grafana/loki:3.6.8

COPY --chown=loki:loki config.yaml /etc/loki/config.yaml

USER loki

EXPOSE 3100

ENTRYPOINT ["/usr/bin/loki"]

CMD ["-config.file=/etc/loki/config.yaml", "-config.expand-env=true"]

