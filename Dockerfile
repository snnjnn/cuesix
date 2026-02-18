# ==========================================================
# APISIX + GeoIP2 dynamic module + sixpack binary
# Multi-stage build
# ==========================================================

ARG APISIX_VERSION=3.14.1-debian
ARG GOLANG_VERSION=1.25

############################
# Stage 1 — Build GeoIP2 module
############################
FROM docker.io/apache/apisix:${APISIX_VERSION} AS geoip-builder

USER root

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        wget \
        git \
        libmaxminddb-dev \
        libpcre3-dev \
        zlib1g-dev \
        libssl-dev \
        perl \
        make && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /tmp

# Detect OpenResty version
RUN OPENRESTY_VERSION=$(nginx -v 2>&1 | cut -d/ -f2) && \
    echo "Detected OpenResty version: ${OPENRESTY_VERSION}" && \
    wget -q https://openresty.org/download/openresty-${OPENRESTY_VERSION}.tar.gz && \
    tar -xzf openresty-${OPENRESTY_VERSION}.tar.gz

# Clone GeoIP2 module
RUN git clone --depth 1 https://github.com/leev/ngx_http_geoip2_module.git

# Build dynamic module only (no full OpenResty build)
RUN set -eux; \
    CC_OPT=$(nginx -V 2>&1 | awk -F"'" '/--with-cc-opt=/{print $2}'); \
    LD_OPT=$(nginx -V 2>&1 | awk -F"'" '/--with-ld-opt=/{print $2}'); \
    cd openresty-*; \
    cd bundle/nginx-*; \
    ./configure \
        --with-compat \
        --with-cc-opt="${CC_OPT}" \
        --with-ld-opt="${LD_OPT}" \
        --add-dynamic-module=../../../ngx_http_geoip2_module; \
    make modules; \
    cp objs/ngx_http_geoip2_module.so /tmp/ngx_http_geoip2_module.so

############################
# Stage 2 — Build sixpack binary
############################
FROM docker.io/golang:${GOLANG_VERSION} AS sixpack-builder

WORKDIR /src
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOEXPERIMENT=jsonv2 \
    go build -trimpath -ldflags="-s -w" -o /out/sixpack ./cmd/sixpack

############################
# Stage 3 — Download Lua dependency
############################
FROM docker.io/apache/apisix:${APISIX_VERSION} AS lua-downloader

USER root

RUN apt-get update && \
    apt-get install -y --no-install-recommends git && \
    rm -rf /var/lib/apt/lists/*

RUN git clone --depth 1 \
    https://github.com/anjia0532/lua-resty-maxminddb.git \
    /maxminddb

############################
# Stage 4 — Final Runtime Image
############################
FROM docker.io/apache/apisix:${APISIX_VERSION}

USER root

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        jq \
        libmaxminddb0 && \
    rm -rf /var/lib/apt/lists/*

# Optional compatibility symlink
RUN ln -sf /usr/lib/x86_64-linux-gnu/libmaxminddb.so.0 \
           /usr/lib/x86_64-linux-gnu/libmaxminddb.so

# Copy GeoIP2 module
COPY --from=geoip-builder /tmp/ngx_http_geoip2_module.so \
    /usr/local/apisix/modules/ngx_http_geoip2_module.so

RUN chmod 644 /usr/local/apisix/modules/ngx_http_geoip2_module.so

# Copy Lua maxminddb helper
COPY --from=lua-downloader \
    /maxminddb/lib/resty/maxminddb.lua \
    /usr/local/apisix/lualib/resty/maxminddb.lua

# Copy sixpack binary
COPY --from=sixpack-builder /out/sixpack /usr/local/bin/sixpack

# Ensure permissions
RUN chmod -R a+rX /usr/local/apisix/deps

USER apisix

# ==========================================================
# Build example (LXC needs --network=host):
#
# sudo podman build --network=host -t localhost/apisix:latest .
# ==========================================================
