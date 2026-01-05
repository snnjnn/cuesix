ARG APISIX_VERSION=3.14.1-debian
ARG GOLANG_VERSION=1.25

FROM docker.io/golang:${GOLANG_VERSION} AS builder

WORKDIR /src
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOEXPERIMENT=jsonv2 \
    go build -trimpath -ldflags="-s -w" -o /out/cuesix ./cmd/cuesix

FROM docker.io/apache/apisix:${APISIX_VERSION}

USER root

RUN DEBIAN_FRONTEND=noninteractive apt-get update \
    && apt-get install -y --no-install-recommends jq \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/cuesix /usr/local/bin/cuesix

# Avoid problems copying the /usr/local/apisix folder
RUN chmod -R a+rX /usr/local/apisix/deps

USER apisix
ENTRYPOINT ["cuesix"]
