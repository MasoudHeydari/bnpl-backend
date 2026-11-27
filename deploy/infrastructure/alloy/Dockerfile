# dpay Central Grafana Alloy — config baked in (Hamravesh rejects host bind mounts).
#
# Pin matches Edge Alloy in the BNPL runtime-development-alloy image.
#
#   docker build -t registry.hamdocker.ir/parstis-technology/dpay-alloy:v0.1.0 \
#     -f deploy/infrastructure/alloy/Dockerfile deploy/infrastructure/alloy

FROM docker.arvancloud.ir/grafana/alloy:v1.22.0

COPY --chmod=644 config.alloy /etc/alloy/config.alloy

# Official image runs as user `alloy` (UID 473). Persist positions/WAL under
# /var/lib/alloy/data (Compose volume dpay_alloy_data).
USER alloy

EXPOSE 3500 12345

ENTRYPOINT ["/bin/alloy"]
CMD ["run", "--server.http.listen-addr=0.0.0.0:12345", "--storage.path=/var/lib/alloy/data", "/etc/alloy/config.alloy"]
