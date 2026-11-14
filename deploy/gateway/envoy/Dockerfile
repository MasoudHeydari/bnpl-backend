FROM envoyproxy/envoy:v1.40.2
USER root
COPY envoy.yaml /etc/envoy/envoy.template.yaml
COPY entrypoint.sh /usr/local/bin/dpay-envoy-entrypoint.sh
COPY certs/tls.crt /etc/envoy/certs/tls.crt
COPY certs/tls.key /etc/envoy/certs/tls.key
RUN chmod 644 /etc/envoy/envoy.template.yaml /etc/envoy/certs/tls.crt \
    && chmod 600 /etc/envoy/certs/tls.key \
    && chmod 755 /usr/local/bin/dpay-envoy-entrypoint.sh
CMD ["/bin/sh", "/usr/local/bin/dpay-envoy-entrypoint.sh"]
