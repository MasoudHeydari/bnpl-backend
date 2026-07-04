FROM golang:1.26.2-alpine3.23 AS dependencies
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

FROM alpine:3.23 AS base
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
FROM base AS runtime
ENV CONFIG_FILE=configs/runtime.yaml
COPY --from=runtime-builder --chown=bnpluser:bnpluser /out/bnpl ./bnpl
COPY --chown=bnpluser:bnpluser configs/runtime.yaml ./configs/runtime.yaml
COPY --chmod=755 --chown=bnpluser:bnpluser scripts/entrypoint.sh ./entrypoint.sh
EXPOSE 8000 8443
CMD ["./entrypoint.sh"]
