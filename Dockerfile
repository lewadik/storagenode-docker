ARG DOCKER_PLATFORM
ARG DOCKER_ARCH
ARG GO_DOCKER_PLATFORM

FROM --platform=${GO_DOCKER_PLATFORM:-linux/amd64} golang:1.23-alpine AS builder
ARG CGO_ENABLED=0
WORKDIR /app
COPY go.mod go.sum ./
COPY ./supervisor ./supervisor
COPY ./cmd/supervisor ./cmd/supervisor
RUN mkdir -p /app/bin
ENV GOCACHE=/root/.cache/go-build
RUN --mount=type=cache,target="/root/.cache/go-build" go build -o ./bin/supervisor ./cmd/supervisor

FROM --platform=${DOCKER_PLATFORM:-linux/amd64} ${DOCKER_ARCH:-amd64}/debian:bookworm-slim
ARG VERSION_SERVER_URL
ARG SUPERVISOR_SERVER
ENV GOARCH=${GOARCH:-amd64} \
    VERSION_SERVER_URL=${VERSION_SERVER_URL:-https://version.storj.io} \
    SUPERVISOR_SERVER=${SUPERVISOR_SERVER:-unix}

RUN apt-get update
RUN apt-get install -y --no-install-recommends ca-certificates
RUN update-ca-certificates

COPY docker/ /

RUN mkdir -p /app/bin
COPY --from=builder /app/bin/supervisor /app/bin/supervisor

EXPOSE 28967
EXPOSE 14002

WORKDIR /app
ENTRYPOINT ["/entrypoint"]

ENV ADDRESS="" \
    EMAIL="" \
    WALLET="" \
    STORAGE="2.0TB" \
    SETUP="false" \
    AUTO_UPDATE="true" \
    LOG_LEVEL="" \
    BINARY_STORE_DIR="/app/config/bin"
