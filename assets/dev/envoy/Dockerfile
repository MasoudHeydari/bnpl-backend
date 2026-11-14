# dpay Envoy gateway — own image with baked config + TLS (no bind mounts, no ENV base64).
#
# Build from this directory (Dockerfile context = assets/dev/envoy):
#   docker build -t registry.hamdocker.ir/parstis-technology/dpay-envoy:v0.1.0 \
#     -f assets/dev/envoy/Dockerfile assets/dev/envoy
#
# Requires certs/tls.crt and certs/tls.key next to this Dockerfile.
FROM envoyproxy/envoy:v1.40.2

USER root

COPY envoy.yaml /etc/envoy/envoy.yaml
COPY certs/tls.crt /etc/envoy/certs/tls.crt
COPY certs/tls.key /etc/envoy/certs/tls.key

RUN chmod 644 /etc/envoy/envoy.yaml /etc/envoy/certs/tls.crt \
	&& chmod 600 /etc/envoy/certs/tls.key

CMD ["envoy", "-c", "/etc/envoy/envoy.yaml", "--log-level", "info"]
