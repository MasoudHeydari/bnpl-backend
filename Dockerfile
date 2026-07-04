FROM golang:1.26.2-alpine3.23 AS builder

ARG APP_NAME=bnpl

WORKDIR /app

RUN apk add --no-cache git

ENV GOPROXY=https://goproxy.io,https://proxy.golang.org,direct

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build \
    -tags="go_json nomsgpack production" \
    -trimpath \
    -ldflags='-s -w -extldflags "-static"' \
    -o /out/${APP_NAME} ./cmd/bnpl


FROM alpine:3.23

LABEL MAINTAINER="Author dev.masoud1997@gmail.com>"

ARG APP_NAME=bnpl
ARG APP_USER=bnpluser
ARG HTTP_PORT=8000
ARG HTTPS_PORT=8443

ENV APP_NAME=${APP_NAME}
ENV CONFIG_FILE=configs/config.yaml

RUN apk add --no-cache tzdata ca-certificates && \
    ln -snf /usr/share/zoneinfo/Asia/Tehran /etc/localtime && \
    echo "Asia/Tehran" > /etc/timezone && \
    adduser -D ${APP_USER}

WORKDIR /home/${APP_USER}/app

COPY --from=builder /out/${APP_NAME} ./
COPY --from=builder /app/configs ./configs
COPY --from=builder /app/scripts/entrypoint.sh ./entrypoint.sh

RUN chmod +x ./entrypoint.sh && \
    chown -R ${APP_USER}:${APP_USER} /home/${APP_USER}/app

USER ${APP_USER}

EXPOSE ${HTTP_PORT}
EXPOSE ${HTTPS_PORT}

CMD ["./entrypoint.sh"]
