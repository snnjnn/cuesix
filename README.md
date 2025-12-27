# cuesix

`cuesix` compiles APISIX standalone configuration from YAML fragments, merges them with APISIX-aware rules, and can validate/reload APISIX.

## Modes

- One-shot (default): compile and print YAML to stdout, no cache/validate/reload.
- Server mode (`--serve`): expose `POST /compile` and run the compile pipeline in the background.

## Usage

One-shot:

```bash
cuesix --input ./configs --input ./more-configs
```

Server mode:

```bash
cuesix --serve \
  --listen :8080 \
  --input ./configs \
  --apisix-config /usr/local/apisix/conf/config.yaml \
  --apisix-url http://127.0.0.1:9180/apisix/admin/configs?reload=true
```

## Flags and environment variables

All flags can be provided as environment variables.

- `--serve` / `CUESIX_SERVE` (bool)
- `--listen` / `CUESIX_LISTEN`
- `--input` (repeatable) / `CUESIX_INPUT_DIRS` (comma-separated)
- `--cooldown` / `CUESIX_COOLDOWN`
- `--apisix-config` / `CUESIX_APISIX_CONFIG`
- `--apisix-url` / `CUESIX_APISIX_URL`
- `--apisix-api-key` / `CUESIX_APISIX_API_KEY`
- `--reload-method` / `CUESIX_RELOAD_METHOD`
- `--retry-max` / `CUESIX_RETRY_MAX`
- `--retry-initial` / `CUESIX_RETRY_INITIAL`
- `--retry-max-delay` / `CUESIX_RETRY_MAX_DELAY`
- `--retry-multiplier` / `CUESIX_RETRY_MULTIPLIER`
- `--plugin-ssl-path` (repeatable) / `CUESIX_PLUGIN_SSL_PATHS` (comma-separated)

## Pipeline (server mode)

1. Compile YAML fragments into a merged config.
2. Apply optional plugins to the merged map.
3. Cache compares content and writes a deterministic temp YAML file when changed (with a trailing `#END` line).
4. Validate with `apisix test`.
5. Replace config and call the APISIX admin reload endpoint.

## Build

```bash
docker build -t cuesix:local .
```

## Layout

See `AGENTS.md` for module responsibilities and documentation requirements.
