set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

#GOEXPERIMENT := "jsonv2"
GOEXPERIMENT := ""

swagger: 
    # Build the OpenAPI spec. Prerequisite:
    # go install github.com/swaggo/swag/cmd/swag@latest
    # Ensure $GOBIN or $GOPATH/bin is in PATH so `swag` is available.
    swag init -g cmd/sixpack/control/api.go --parseInternal --output cmd/sixpack/docs

app:
    # Build the web UI. Prerequisite:
    # cd internal/app/assets && npm install --ignore-scripts
    rm internal/app/assets/dist/*.ttf || true
    rm internal/app/assets/dist/*.woff2 || true
    npm --prefix internal/app/assets run build

build: swagger app
    GOEXPERIMENT={{GOEXPERIMENT}} CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o ./bin/sixpack ./cmd/sixpack

tag TAG: swagger app
    docker build -t {{TAG}} .

test:
    GOEXPERIMENT={{GOEXPERIMENT}} go test ./...

cover:
    GOEXPERIMENT={{GOEXPERIMENT}} go test ./... -coverprofile=coverage.out
    GOEXPERIMENT={{GOEXPERIMENT}} go tool cover -func=coverage.out

buffering-e2e:
    ./src/buffering/tests/run-e2e.sh
