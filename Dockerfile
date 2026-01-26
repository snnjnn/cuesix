ARG APISIX_VERSION=3.14.1-debian
ARG GOLANG_VERSION=1.25

# Build sixpack app
# ----------------
FROM docker.io/golang:${GOLANG_VERSION} AS builder

WORKDIR /src
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOEXPERIMENT=jsonv2 \
    go build -trimpath -ldflags="-s -w" -o /out/sixpack ./cmd/sixpack

# Download lua modules
# --------------------
FROM docker.io/apache/apisix:${APISIX_VERSION} AS downloader

USER root

RUN DEBIAN_FRONTEND=noninteractive apt-get update \
    && apt-get install -y --no-install-recommends \
      git curl wget

RUN git clone --depth 1 \
  https://github.com/anjia0532/lua-resty-maxminddb.git \
  /maxminddb

# Assemble apisix image
# ---------------------
FROM docker.io/apache/apisix:${APISIX_VERSION}

USER root

RUN DEBIAN_FRONTEND=noninteractive apt-get update \
    && apt-get install -y --no-install-recommends \
      jq libmaxminddb0 \
    && rm -rf /var/lib/apt/lists/*

# Link .so library
RUN ln -s /usr/lib/x86_64-linux-gnu/libmaxminddb.so.0 \
          /usr/lib/x86_64-linux-gnu/libmaxminddb.so

COPY --from=downloader /maxminddb/lib/resty/maxminddb.lua /usr/local/apisix/lualib/resty/maxminddb.lua
COPY --from=builder    /out/sixpack /usr/local/bin/sixpack

# Avoid problems copying the /usr/local/apisix folder
RUN chmod -R a+rX /usr/local/apisix/deps

USER apisix
ENTRYPOINT ["sixpack"]
