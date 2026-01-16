## Retrieval of schema fixtures

```bash
curl http://apisic-control-api/v1/schema | jq -r > test/apisix_schema.json
go run ../../cmd/schema --url https://apisix-pre.iotplatform.telefonica.com | jq > test/processed_schema.json
go run ../../cmd/schema --url https://apisix-pre.iotplatform.telefonica.com --loose | jq > test/loose_processed_schema.json
```

## Playground compilation with esbuild

```bash
npx esbuild app/src/index.js --bundle --outdir=app/dist --splitting --format=esm --loader:.svg=file
```

Or:

```bash
npm install --save-dev esbuild
go generate ./...
```
