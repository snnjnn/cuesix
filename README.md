# sixpack

`sixpack` compiles Apache APISIX standalone configuration from YAML fragments, applies APISIX-aware merge rules, and can validate/reload APISIX.

## APISIX standalone config in a nutshell

APISIX has a home directory (default `/usr/local/apisix`), where `conf/` lives. The standalone mode uses two different configuration files inside the `conf/` folder:

- `config.yaml`: static runtime config (ports, role, admin settings, etc).
- `apisix.yaml` or `apisix.json`: dynamic config (routes, services, consumers, plugins).

APISIX determines whether the dynamic config is YAML or JSON via `config.yaml`:

```yaml
deployment:
  role: data_plane
  role_data_plane:
    # config provider defines if thee dynamic config
    # is json or yaml. By default, it is yaml.
    config_provider: json|yaml
```

APISIX also supports profiles via the `APISIX_PROFILE` environment variable. When set, APISIX loads `config-<profile>.yaml>` and `apisix-<profile>.[yaml|json]`, instead of `comfig.yaml` and `apisix.[yaml|json]`.

## What sixpack does

Sixpack builds a unified dynamic config file, `apisix.[yaml|json]`, by reading many YAML configuration fragments from a list of input folders.

Configuration files for apisix look like this:

```yaml
ssls:
  - id: 1
    cert: |-
      -----BEGIN CERTIFICATE-----
      ...
      -----END CERTIFICATE-----
    key: |-
      -----BEGIN PRIVATE KEY-----
      ...
      -----END PRIVATE KEY-----
    snis:
      - public.domain.org

routes:
  - uri: /*
    plugins:
      redirect:
        http_to_https: true
    upstream:
        nodes:
            "backend.url:3000": 1
        type: roundrobin
```

Sixpack can merge many of these files, by applying a set of **apisix-specific merge rules**:

- Most apisix lists (like `ssls` or `routes`) can be merged by a key like **id**: lists containing objects with different ids can just be concatenated.
- Some lists, like `consumers`, have a different merge `key` (`name` instead of `id`).
- Tipically, two lists cannot be merged if they both contain an item with the same key (`id`, `user`, whatever).
- However, some other lists **can** be merged. For instance, `consumers` lists that contain the same `consumer` can we merged if the `consumer` in both lists only differ in the `credentials` attribute. The consumer's `credentials` attribute is a sublist that will itself be merged.

The full set of merge rules for lists is maintained in the [compiler.go](internal/compiler/compiler.go) file.

By default, sixpack will generate an aggregated `apisix.json` (or `apisix-${APISIX_PROFILE}.json`). It will not honor the value of `deployment.role_data_place.config_provider`. If you need the output to be yaml, use the `--plugin-yaml` flag.

## Features

Besides merging files, sixpack implements some quality-of-life features that expand the expressivity of the input yaml files

### Certificate inlining

The `--plugin-ssl-path` (repeatable) flag activates the SSL plugin. This plugin scans `ssls` entries for `$secret://file/...` or `$secret://acme/...` values in both `cert`/`key` and `certs`/`keys`.

- If a certificate or key URL is `$secret://file/...`, it searches for the given file name in the folders specified with the `--plugin-ssl-path` flag, and embeds them into the yaml. Missing files are replaced with the fallback certificate/key configured via `--plugin-ssl-fallback-cert`/`--plugin-ssl-fallback-key`.

For example, a config snippet like:

```yaml
ssls:
  - cert: "$secret://file/tls-domain-name.pem"
```

Will get replaced by:

```yaml
ssls:
  - cert:
      -----BEGIN CERTIFICATE-----
      ...
      -----END CERTIFICATE-----
```

- If the certificate URL is `$secret://acme/...`, it will try to generate a new ACME certificate.
  - Acme certificates and SANs is a complicated story, so this mode only works when the `ssls` entry has a single `sni`.
  - The `key` entry is ignored, it is overriden with the acme key.
  - If ACME is unavailable or fails, the fallback certificate/key is used.

If an `ssls` entry has malformed `cert`/`key` or `certs`/`keys` (wrong types or list length mismatch), it is left untouched.

### Config-wide transformations

Sometimes you might need to post-process `apisix` snippets. For instance, adding a particular plugin to all routes that match a given criteria.

The `jq` plugin allows you to embed jq-based transformations into your config snippets. Those transformations will be applied across the whole merged config file.

For example, say you need to enable basic auth for all routes that begin with `/admin/`. After enabling transformations with the `--plugin-jq`  flag, you can use the following yaml snippet:

```yaml
jq:
  - id: "admin-basic-auth"
    # Add basic-auth plugin to all routes that start with `/admin`
    prio: 10
    expr: |
      .routes |= (map(
        if ((.uri? // "") | startswith("/admin/"))
           or ((.uris? // []) | any(startswith("/admin/")))
        then .plugins = ((.plugins // {}) + {"basic-auth": {}})
        else .
        end
      ))
```

The transformations are merged and applied in descending priority (`prio`) order. The order within the same priority is undefined.

Jq transformations must always return the full config object, they cannot be partial.

### YAML provider

By default, apisix produces json output, You can use the `--plugin-yaml` flag to make it produce yaml instead.

Apisix requires the `apisix.yaml` to terminate with the comment `#END`. The sixpack yaml plugin takes care of this.

### Validation

When running in server mode, sixpack validates the files produced, before replacing the apisix config file.

Validation uses the `apisix test` command and is automatic. It works for json and yaml files, and it does not require any additional flag.

When validation fails, an error message is logged. If it doesn't fail, the config file is replaced.

To enable schema validation from APISIX, you need to provide the URL of the apisix control endpoint with the flag `--apisix-control-url http://127.0.0.1:9090`.

### Schema endpoint

In server mode, the metrics server also exposes `GET /schema`. It returns a
complete JSON Schema for APISIX standalone configs, synthesized from the live
control‑API schema and the standalone top‑level mapping.

Implementation details live in `internal/schema/README.md`.

### Schema CLI and fixtures

The `cmd/schema` CLI downloads the live APISIX schema and prints the normalized
schema to stdout. It defaults to the strict variant; use `--loose` to keep
APISIX's permissive ID rules.

```bash
go run ./cmd/schema --url http://127.0.0.1:9090 > internal/schema/processed_schema.json
go run ./cmd/schema --url http://127.0.0.1:9090 --loose > internal/schema/loose_processed_schema.json
```

To refresh the raw schema fixture used by tests:

```bash
curl -s http://127.0.0.1:9090/v1/schema > internal/schema/apisix_schema.json
```

The `internal/schema` tests compare the processed strict schema against
`internal/schema/processed_schema.json` and the loose schema against
`internal/schema/loose_processed_schema.json`.

## Run modes

Standalone (default): compiles and prints the merged config to stdout. Can optionally use schema validation if `--apisix-use-schema` is provided and `--apisix-control-url` points to the control URL of a running apisix instance. Will not do post-build validation (`apisix test`)

Server mode (`sixpack serve`): exposes `POST /compile`, `GET /live`, and `GET /ready`, runs the pipeline, validates the result, and writes the config file on success. `/ready` returns 200 only after a successful config has been written at least once. Certmagic automatically manages its own HTTP server for ACME challenges on the configured port.

## Flags and environment variables

All flags can be provided as environment variables.

Input and runtime mode:
- `--listen` / `SIXPACK_LISTEN`: listen address for server mode (default `127.0.0.1:8080`).
- `--metrics` / `SIXPACK_METRICS_LISTEN`: listen address for `/metrics` (empty disables).
- `--server-read-header-timeout` / `SIXPACK_SERVER_READ_HEADER_TIMEOUT`: HTTP server read header timeout (default `5s`).
- `--server-read-timeout` / `SIXPACK_SERVER_READ_TIMEOUT`: HTTP server read timeout (default `10s`).
- `--server-write-timeout` / `SIXPACK_SERVER_WRITE_TIMEOUT`: HTTP server write timeout (default `10s`).
- `--server-idle-timeout` / `SIXPACK_SERVER_IDLE_TIMEOUT`: HTTP server idle timeout (default `60s`).
- `--server-shutdown-timeout` / `SIXPACK_SERVER_SHUTDOWN_TIMEOUT`: HTTP server shutdown timeout (default `10s`).
- `--input` (repeatable) / `SIXPACK_INPUT_DIRS` (comma-separated): input directories with YAML fragments.
- `--cooldown` / `SIXPACK_COOLDOWN`: minimum delay between queued compile runs.
- `--dry-run` / `SIXPACK_DRY_RUN` (bool): run pipeline without writing config.

APISIX paths and validation:
- `--apisix-home` / `SIXPACK_APISIX_HOME`: APISIX home directory (default `/usr/local/apisix`).
- `--mirror-dir` / `SIXPACK_MIRROR_DIR`: optional mirror directory for validation; if empty, sixpack creates a temp mirror.
- `--keep-mirror` / `SIXPACK_KEEP_MIRROR`: do not clean and re-populate the mirror folder on startup.
- `--validation-timeout` / `SIXPACK_VALIDATION_TIMEOUT`: timeout for `apisix test` validation.
- `--apisix-use-schema` / `SIXPACK_APISIX_USE_SCHEMA`: validate config snippets against the live APISIX schema (requires `--apisix-control-url`).

APISIX Control API:
- `--apisix-control-url` / `SIXPACK_APISIX_CONTROL_URL`: APISIX Control API base URL (default `http://127.0.0.1:9090`).
- `--apisix-api-key` / `SIXPACK_APISIX_API_KEY`: Control API key for schema requests.
- `--apisix-api-timeout` / `SIXPACK_APISIX_API_TIMEOUT`: timeout for Control API HTTP requests.
- `--retry-max` / `SIXPACK_RETRY_MAX`: number of API request retries on failure.
- `--retry-initial` / `SIXPACK_RETRY_INITIAL`: initial backoff before the first retry.
- `--retry-max-delay` / `SIXPACK_RETRY_MAX_DELAY`: cap for retry backoff.
- `--retry-multiplier` / `SIXPACK_RETRY_MULTIPLIER`: backoff multiplier between retries.

Plugins:
- `--plugin-ssl` / `SIXPACK_PLUGIN_SSL`: enable ssl pre-render plugin (required to process `$secret://acme/` without certmagic).
- `--plugin-jq` / `SIXPACK_PLUGIN_JQ`: enable jq post-render plugin.
- `--plugin-jq-timeout` / `SIXPACK_PLUGIN_JQ_TIMEOUT`: timeout for jq transforms.
- `--plugin-ssl-path` (repeatable) / `SIXPACK_PLUGIN_SSL_PATHS` (comma-separated): search paths for SSL certificate files.
- `--plugin-ssl-acme-timeout` / `SIXPACK_PLUGIN_SSL_ACME_TIMEOUT`: timeout for ssl plugin ACME requests (default `10s`, must be positive).
- `--plugin-ssl-fallback-cert` / `SIXPACK_PLUGIN_SSL_FALLBACK_CERT`: ssl plugin fallback certificate path (default `${APISIX_HOME}/conf/cert/ssl_PLACE_HOLDER.crt`).
- `--plugin-ssl-fallback-key` / `SIXPACK_PLUGIN_SSL_FALLBACK_KEY`: ssl plugin fallback key path (default `${APISIX_HOME}/conf/cert/ssl_PLACE_HOLDER.key`).
- `--plugin-env` / `SIXPACK_PLUGIN_ENV`: per-directory env file name used for APISIX `${{ VAR }}` substitutions in input snippets.
- `--plugin-yaml` / `SIXPACK_PLUGIN_YAML`: enable YAML post-render plugin (use when `config_provider: yaml`).

Certmagic (serve only):
- `--certmagic` / `SIXPACK_CERTMAGIC` (bool): enable certmagic ACME manager.
- `--certmagic-provider` (repeatable) / `SIXPACK_CERTMAGIC_PROVIDERS` (comma-separated): provider specs (`name|email|ca`).
- `--certmagic-default-provider` / `SIXPACK_CERTMAGIC_DEFAULT_PROVIDER`: default provider name.
- `--certmagic-data-dir` / `SIXPACK_CERTMAGIC_DATA_DIR`: certmagic data directory (required when enabled).
- `--certmagic-challenge-port` / `SIXPACK_CERTMAGIC_CHALLENGE_PORT`: HTTP-01 challenge port (default `8080`).
- `--certmagic-timeout` / `SIXPACK_CERTMAGIC_TIMEOUT`: default certificate obtain timeout.
- `--certmagic-watch-interval` / `SIXPACK_CERTMAGIC_WATCH_INTERVAL`: refresh interval for certmagic certificate updates (default `1h`).
- `--certmagic-untracked-interval` / `SIXPACK_CERTMAGIC_UNTRACKED_INTERVAL`: interval for removing untracked certmagic entries (default `24h`).
- `--certmagic-untracked-grace` / `SIXPACK_CERTMAGIC_UNTRACKED_GRACE`: grace period for removing untracked certmagic entries (default `168h`).
- `--certmagic-cleanup-interval` / `SIXPACK_CERTMAGIC_EXPIRED_INTERVAL`: interval for removing expired certmagic entries (default `24h`).
- `--certmagic-expired-grace` / `SIXPACK_CERTMAGIC_EXPIRED_GRACE`: grace period for removing expired certmagic entries (default `125h`).

When an ACME certificate cannot be obtained, sixpack will use the SSL plugin fallback certificate to keep the `ssls` entry valid. Certmagic keeps retrying, and when a certificate becomes available sixpack triggers a new compile cycle.

## Usage

Standalone:

```bash
sixpack compile --input ./configs --input ./more-configs
```

Server mode:

```bash
sixpack serve \
  --listen :8080 \
  --metrics :9090 \
  --input ./configs \
  --apisix-home /usr/local/apisix \
  --apisix-control-url http://127.0.0.1:9090
```

## Build

```bash
docker build -t sixpack:latest .
```

## Layout

See `AGENTS.md` for module responsibilities and documentation requirements.
