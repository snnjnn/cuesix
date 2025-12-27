FROM golang:1.25 AS builder

WORKDIR /src
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/cuesix ./cmd/cuesix

FROM apache/apisix:latest

COPY --from=builder /out/cuesix /usr/local/bin/cuesix

ENTRYPOINT ["cuesix"]
