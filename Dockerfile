# Base images are pulled through the ArvanCloud Docker Hub mirror
# (https://docker.arvancloud.ir) so builds do not depend on direct Docker Hub
# access. This matches the other deployment Dockerfiles in this repository.
FROM docker.arvancloud.ir/golang:1.26.2-alpine3.23 AS dependencies
WORKDIR /app
RUN apk add --no-cache git
ENV GOPROXY=https://goproxy.io,https://proxy.golang.org,direct
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY pkg ./pkg
COPY internal ./internal
COPY gen ./gen

FROM dependencies AS runtime-builder
ARG BUILD_ENV=production
RUN --mount=type=cache,target=/root/.cache/go-build case "$BUILD_ENV" in development|production) ;; *) exit 1;; esac && \
    CGO_ENABLED=0 go build -tags="go_json nomsgpack ${BUILD_ENV}" -trimpath \
    -ldflags='-s -w -extldflags "-static"' -o /out/bnpl ./cmd/bnpl

FROM dependencies AS dbadmin-builder
ARG BUILD_ENV=production
RUN --mount=type=cache,target=/root/.cache/go-build case "$BUILD_ENV" in development|production) ;; *) exit 1;; esac && \
    CGO_ENABLED=0 go build -tags="go_json nomsgpack ${BUILD_ENV}" -trimpath \
    -ldflags='-s -w -extldflags "-static"' -o /out/bnpl-dbadmin ./cmd/bnpl-dbadmin

# Static grpcurl for the development runtime image only.
FROM dependencies AS grpcurl-builder
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go install github.com/fullstorydev/grpcurl/cmd/grpcurl@v1.9.3

FROM docker.arvancloud.ir/alpine:3.23 AS base
RUN apk add --no-cache tzdata ca-certificates && \
    ln -snf /usr/share/zoneinfo/Asia/Tehran /etc/localtime && \
    echo "Asia/Tehran" > /etc/timezone && adduser -D bnpluser
WORKDIR /home/bnpluser/app
USER bnpluser

FROM base AS dbadmin
ENV CONFIG_FILE=configs/dbadmin.yaml
COPY --from=dbadmin-builder --chown=bnpluser:bnpluser /out/bnpl-dbadmin ./bnpl-dbadmin
COPY --chown=bnpluser:bnpluser configs/dbadmin.yaml ./configs/dbadmin.yaml
COPY --chown=bnpluser:bnpluser assets/db/migrations ./assets/db/migrations
ENTRYPOINT ["./bnpl-dbadmin"]
CMD ["--help"]

# The default final image retains the existing Runtime entrypoint behavior.
# Production/runtime builds intentionally omit outbound gRPC client PEMs.
FROM base AS runtime
ENV CONFIG_FILE=configs/runtime.yaml
COPY --from=runtime-builder --chown=bnpluser:bnpluser /out/bnpl ./bnpl
COPY --chown=bnpluser:bnpluser configs/runtime.yaml ./configs/runtime.yaml
COPY --chmod=755 --chown=bnpluser:bnpluser scripts/runtime/entrypoint.sh ./entrypoint.sh
# Hamravesh one-shot `bnpl-import-trusted-customers` job reads this baked CSV.
COPY --chown=bnpluser:bnpluser deploy/bnpl/hamravesh/trusted-customers.csv \
    ./assets/csv/trusted-customers/trusted-customers.csv
EXPOSE 7080 7080
CMD ["./entrypoint.sh"]

# Development/Hamravesh runtime: bake outbound gRPC mTLS materials into the
# image so Darkoob does not need panel base64 PEMs. Production must keep using
# the `runtime` target (no private key layers).
FROM runtime AS runtime-development

# Create the certificate directory with traversal permission.
USER root

# Network troubleshooting tools for Hamravesh/dev only (absent from production).
# nmap: scan remote open ports (e.g. nmap -Pn -p 1-65535 host).
# grpcurl: probe gRPC services (often with -cacert/-cert/-key under ./certs).
RUN apk add --no-cache \
    bind-tools \
    busybox-extras \
    curl \
    iproute2 \
    iputils \
    mtr \
    netcat-openbsd \
    nmap \
    nmap-ncat \
    openssl \
    tcpdump \
    traceroute \
    vim \
    wget

COPY --from=grpcurl-builder --chmod=755 /go/bin/grpcurl /usr/local/bin/grpcurl

RUN mkdir -p /home/bnpluser/app/certs \
    && chown bnpluser:bnpluser /home/bnpluser/app/certs \
    && chmod 700 /home/bnpluser/app/certs

COPY --chown=bnpluser:bnpluser --chmod=644 \
    deploy/bnpl/hamravesh/certs/ca.crt \
    /home/bnpluser/app/certs/ca.crt

COPY --chown=bnpluser:bnpluser --chmod=644 \
    deploy/bnpl/hamravesh/certs/bnpl-service.crt \
    /home/bnpluser/app/certs/bnpl-service.crt

COPY --chown=bnpluser:bnpluser --chmod=600 \
    deploy/bnpl/hamravesh/certs/bnpl-service.key \
    /home/bnpluser/app/certs/bnpl-service.key

USER bnpluser
