ARG APP_NAME=bnpl

FROM golang:1.26.2-alpine3.23 AS builder

COPY . /app
WORKDIR /app

# Toggle CGO based on your app requirement. CGO_ENABLED=1 for enabling CGO and CGO_ENABLED=0 for disabling.
RUN CGO_ENABLED=0 go build -tags="go_json nomsgpack" -trimpath -ldflags '-s -w -extldflags "-static"' \
-o /app/$APP_NAME ./cmd/bnpl

# Use below if using vendor
# RUN CGO_ENABLED=0 go build -mod=vendor -ldflags '-s -w -extldflags "-static"' -o /app/appbin *.go

FROM alpine:3.23
LABEL MAINTAINER="Author dev.masoud1997@gmail.com>"

ARG APP_NAME=bnpl
ENV APP_NAME_ENV=${APP_NAME}
ARG APP_USER=bnpluser

# Since running as a non-root user, port bindings < 1024 is not possible
ARG PROT_FOR_HTTP=8000
ARG PORT_FOR_HTTPS=8443

# Add new user
RUN adduser -D $APP_USER
USER $APP_USER

COPY --from=builder /app /home/$APP_USER/app

WORKDIR /home/$APP_USER/app

EXPOSE $PROT_FOR_HTTP
EXPOSE $PROT_FOR_HTTPS

CMD ./${APP_NAME_ENV}
