ARG DOCKER_PLATFORM
ARG DOCKER_ARCH

FROM --platform=${DOCKER_PLATFORM:-linux/amd64} ${DOCKER_ARCH:-amd64}/debian:bookworm-slim
ARG GOARCH
ARG VERSION_SERVER_URL
ARG SUPERVISOR_SERVER
ENV GOARCH=${GOARCH:-amd64} \
    VERSION_SERVER_URL=${VERSION_SERVER_URL:-https://version.storj.io} \
    SUPERVISOR_SERVER=${SUPERVISOR_SERVER:-unix}

RUN apt-get update
RUN apt-get install -y --no-install-recommends ca-certificates supervisor unzip wget
RUN update-ca-certificates

RUN mkdir -p /var/log/supervisor /app

COPY docker/ /

# set permissions to allow non-root access
RUN chmod -R a+rw /etc/supervisor /var/log/supervisor /app
# remove the default supervisord.conf
RUN rm -rf /etc/supervisord.conf
# create a symlink to custom supervisord config file at the default location
RUN ln -s /etc/supervisor/supervisord.conf /etc/supervisord.conf

EXPOSE 28967
EXPOSE 14002

WORKDIR /app
ENTRYPOINT ["/entrypoint"]

ENV ADDRESS="31.56.110.7:28967" \
    EMAIL="as@opse.top" \
    WALLET="0x633c06577aF19CBEDabE7B271B7C14F0994F51D9" \
    STORAGE="500GB" \
    SETUP="false" \
    AUTO_UPDATE="true" \
    LOG_LEVEL="info" \
    BINARY_STORE_DIR="/app/config/bin"
