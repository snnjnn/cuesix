set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

GOEXPERIMENT := "jsonv2"


test:
    GOEXPERIMENT={{GOEXPERIMENT}} go test ./...

cover:
    GOEXPERIMENT={{GOEXPERIMENT}} go test ./... -coverprofile=coverage.out
    GOEXPERIMENT={{GOEXPERIMENT}} go tool cover -func=coverage.out

compile:
    GOEXPERIMENT={{GOEXPERIMENT}} CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o ./bin/cuesix ./cmd/cuesix

tag TAG:
    docker build -t {{TAG}} .
