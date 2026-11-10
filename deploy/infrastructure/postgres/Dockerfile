# dpay Postgres — official image + first-boot multi-DB init script baked in.
# Hamravesh rejects host bind mounts; do not mount init-databases.sh at runtime.
#
#   docker build -t registry.hamdocker.ir/parstis-technology/dpay-postgres:v0.1.0 \
#     -f deploy/infrastructure/postgres/Dockerfile deploy/infrastructure/postgres
FROM docker.arvancloud.ir/postgres:18

COPY init-databases.sh /docker-entrypoint-initdb.d/01-init-databases.sh

RUN chmod 755 /docker-entrypoint-initdb.d/01-init-databases.sh
