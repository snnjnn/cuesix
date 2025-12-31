set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

GOEXPERIMENT := "jsonv2"


test:
    GOEXPERIMENT={{GOEXPERIMENT}} go test ./...

cover:
    GOEXPERIMENT={{GOEXPERIMENT}} go test ./... -coverprofile=coverage.out
    GOEXPERIMENT={{GOEXPERIMENT}} go tool cover -func=coverage.out

build:
    GOEXPERIMENT={{GOEXPERIMENT}} CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o ./bin/cuesix ./cmd/cuesix

doc:
    # Needs pkgsite installed
    # go install golang.org/x/pkgsite/cmd/pkgsite@latest
    # by default installed to $HOME/go/bin
    pkgsite -open .

tag TAG:
    docker build -t {{TAG}} .
