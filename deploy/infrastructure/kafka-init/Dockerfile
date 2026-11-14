FROM docker.arvancloud.ir/apache/kafka:4.3.1
COPY --chmod=755 init-topics.sh /opt/kafka/init-topics.sh
ENTRYPOINT ["/bin/bash", "/opt/kafka/init-topics.sh"]
