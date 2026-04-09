## Schema fixtures

Este paquete incluye lógica de normalización/validación de esquema y usa fixtures versionados en `internal/schema/test`.

## Regenerar fixtures

Desde la raíz del repo:

```bash
curl -s http://127.0.0.1:9090/v1/schema > internal/schema/test/apisix_schema.json
go run ./cmd/schema --url http://127.0.0.1:9090 > internal/schema/test/processed_schema.json
go run ./cmd/schema --url http://127.0.0.1:9090 --loose > internal/schema/test/loose_processed_schema.json
```

`cmd/schema` está limitado intencionadamente a generación de fixtures: descarga `/v1/schema` de APISIX y escribe en `stdout` el esquema JSON normalizado.

## Notas

- `processed_schema.json`: variante estricta.
- `loose_processed_schema.json`: variante laxa.
- `apisix_schema.json`: respuesta cruda de `GET /v1/schema`.

## Playground compilation with esbuild

```bash
npx esbuild app/src/index.js --bundle --outdir=assets/dist --splitting --format=esm --loader:.svg=file
```

Or:

```bash
npm install --save-dev esbuild
go generate ./...
```
