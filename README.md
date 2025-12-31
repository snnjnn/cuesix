# cuesix

`cuesix` compiles Apache APISIX standalone configuration from YAML fragments, applies APISIX-aware merge rules, and can validate/reload APISIX.

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

## What cuesix does

Cuesix builds a unified dynamic config file, `apisix.[yaml|json]`, by reading many YAML configuration fragments from a list of input folders.

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

Cuesix can merge many of these files, by applying a set of **apisix-specific merge rules**:

- Most apisix lists (like `ssls` or `routes`) can be merged by a key like **id**: lists containing objects with different ids can just be concatenated.
- Some lists, like `consumers`, have a different merge `key` (`name` instead of `id`).
- Tipically, two lists cannot be merged if they both contain an item with the same key (`id`, `user`, whatever).
- However, some other lists **can** be merged. For instance, `consumers` lists that contain the same `consumer` can we merged if the `consumer` in both lists only differ in the `credentials` attribute. The consumer's `credentials` attribute is a sublist that will itself be merged.

The full set of merge rules for lists is maintained in the [compiler.go](internal/compiler/compiler.go) file.

By default, cuesix will generate an aggregated `apisix.json` (or `apisix-${APISIX_PROFILE}.json`). It will not honor the value of `deployment.role_data_place.config_provider`. If you need the output to be yaml, use the `--plugin-yaml` flag.

## Features

Besides merging files, cuesix implements some quality-of-life features that expand the expressivity of the input yaml files

### Certificate inlining

The `--plugin-ssl-path` (repeatable) flag activates the SSL plugin. This plugin scans `ssls` entries for `file://...` or `acme://...` values in both `cert`/`key` and `certs`/`keys`.

- If a certificate or key URL is `file://...`, it searches for the given file name in the folders specified with the `--plugin-ssl-path` flag, and embeds them into the yaml. Missing files are replaced with the fallback certificate/key configured via `--plugin-ssl-fallback-cert`/`--plugin-ssl-fallback-key`.

For example, a config snippet like:

```yaml
ssls:
  - cert: "file://tls-domain-name.pem"
```

Will get replaced by:

```yaml
ssls:
  - cert:
      -----BEGIN CERTIFICATE-----
      ...
      -----END CERTIFICATE-----
```

- If the certificate URL is `acme://...`, it will try to generate a new ACME certificate.
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

Apisix requires the `apisix.yaml` to terminate with the comment `#END`. The cuesix yaml plugin takes care of this.

### Validation

When running in server mode, cuesix validates the files produced, before replacing the apisix config file.

Validation uses the `apisix test` command and is automatic. It works for json and yaml files, and it does not require any additional flag.

When validation fails, an error message is logged. If it doesn't fail, the config file is replaced, and optionally, apisix is notified of the change via API.

To enable API reload of APISIX, you need to provide the URL of the apisix control endpoint with the flag `--apisix-url http://127.0.0.1:9180`.

## Run modes

Standalone (default): compiles and prints the merged config to stdout. No validation or reload.

Server mode (`cuesix serve`): exposes `POST /compile`, `GET /live`, and `GET /ready`, runs the pipeline, validates the result, and reloads APISIX on success. `/ready` returns 200 only after a successful reload has been delivered at least once. Certmagic is only available in this mode.

## Flags and environment variables

All flags can be provided as environment variables.

Input and runtime mode:
- `--listen` / `CUESIX_LISTEN`: listen address for server mode (default `127.0.0.1:8080`).
- `--metrics` / `CUESIX_METRICS_LISTEN`: listen address for `/metrics` (empty disables).
- `--server-read-header-timeout` / `CUESIX_SERVER_READ_HEADER_TIMEOUT`: HTTP server read header timeout (default `5s`).
- `--server-read-timeout` / `CUESIX_SERVER_READ_TIMEOUT`: HTTP server read timeout (default `10s`).
- `--server-write-timeout` / `CUESIX_SERVER_WRITE_TIMEOUT`: HTTP server write timeout (default `10s`).
- `--server-idle-timeout` / `CUESIX_SERVER_IDLE_TIMEOUT`: HTTP server idle timeout (default `60s`).
- `--server-shutdown-timeout` / `CUESIX_SERVER_SHUTDOWN_TIMEOUT`: HTTP server shutdown timeout (default `10s`).
- `--input` (repeatable) / `CUESIX_INPUT_DIRS` (comma-separated): input directories with YAML fragments.
- `--cooldown` / `CUESIX_COOLDOWN`: minimum delay between queued compile runs.
- `--dry-run` / `CUESIX_DRY_RUN` (bool): run pipeline without writing config or triggering reload.

APISIX paths and validation:
- `--apisix-home` / `CUESIX_APISIX_HOME`: APISIX home directory (default `/usr/local/apisix`).
- `--mirror-dir` / `CUESIX_MIRROR_DIR`: optional mirror directory for validation; if empty, cuesix creates a temp mirror.
- `--keep-mirror` / `CUESIX_KEEP_MIRROR`: do not clean and re-populate the mirror folder on startup.
- `--validation-timeout` / `CUESIX_VALIDATION_TIMEOUT`: timeout for `apisix test` validation.

Reload behavior:
- `--apisix-url` / `CUESIX_APISIX_URL`: APISIX Admin API base URL (e.g. `http://127.0.0.1:9180`).
- `--apisix-api-key` / `CUESIX_APISIX_API_KEY`: Admin API key for reload requests.
- `--reload-method` / `CUESIX_RELOAD_METHOD`: HTTP method for reload requests (default POST).
- `--reload-timeout` / `CUESIX_RELOAD_TIMEOUT`: timeout for reload HTTP requests.
- `--retry-max` / `CUESIX_RETRY_MAX`: number of reload retries on failure.
- `--retry-initial` / `CUESIX_RETRY_INITIAL`: initial backoff before the first retry.
- `--retry-max-delay` / `CUESIX_RETRY_MAX_DELAY`: cap for retry backoff.
- `--retry-multiplier` / `CUESIX_RETRY_MULTIPLIER`: backoff multiplier between retries.

Plugins:
- `--plugin-ssl` / `CUESIX_PLUGIN_SSL`: enable ssl pre-render plugin (required to process `acme://` without certmagic).
- `--plugin-jq` / `CUESIX_PLUGIN_JQ`: enable jq post-render plugin.
- `--plugin-jq-timeout` / `CUESIX_PLUGIN_JQ_TIMEOUT`: timeout for jq transforms.
- `--plugin-ssl-path` (repeatable) / `CUESIX_PLUGIN_SSL_PATHS` (comma-separated): search paths for SSL certificate files.
- `--plugin-ssl-acme-timeout` / `CUESIX_PLUGIN_SSL_ACME_TIMEOUT`: timeout for ssl plugin ACME requests.
- `--plugin-ssl-fallback-cert` / `CUESIX_PLUGIN_SSL_FALLBACK_CERT`: ssl plugin fallback certificate path (default `${APISIX_HOME}/conf/cert/ssl_PLACE_HOLDER.crt`).
- `--plugin-ssl-fallback-key` / `CUESIX_PLUGIN_SSL_FALLBACK_KEY`: ssl plugin fallback key path (default `${APISIX_HOME}/conf/cert/ssl_PLACE_HOLDER.key`).
- `--plugin-yaml` / `CUESIX_PLUGIN_YAML`: enable YAML post-render plugin (use when `config_provider: yaml`).

Certmagic (serve only):
- `--certmagic` / `CUESIX_CERTMAGIC` (bool): enable certmagic ACME manager.
- `--certmagic-provider` (repeatable) / `CUESIX_CERTMAGIC_PROVIDERS` (comma-separated): provider specs (`name|email|ca`).
- `--certmagic-default-provider` / `CUESIX_CERTMAGIC_DEFAULT_PROVIDER`: default provider name.
- `--certmagic-data-dir` / `CUESIX_CERTMAGIC_DATA_DIR`: certmagic data directory (required when enabled).
- `--certmagic-challenge-addr` / `CUESIX_CERTMAGIC_CHALLENGE_ADDR`: HTTP-01 challenge listen address.
- `--certmagic-timeout` / `CUESIX_CERTMAGIC_TIMEOUT`: default certificate obtain timeout.
- `--certmagic-watch-interval` / `CUESIX_CERTMAGIC_WATCH_INTERVAL`: refresh interval for certmagic certificate updates (default `1h`).
- `--certmagic-untracked-interval` / `CUESIX_CERTMAGIC_UNTRACKED_INTERVAL`: interval for removing untracked certmagic entries (default `24h`).
- `--certmagic-untracked-grace` / `CUESIX_CERTMAGIC_UNTRACKED_GRACE`: grace period for removing untracked certmagic entries (default `168h`).
- `--certmagic-cleanup-interval` / `CUESIX_CERTMAGIC_EXPIRED_INTERVAL`: interval for removing expired certmagic entries (default `24h`).
- `--certmagic-expired-grace` / `CUESIX_CERTMAGIC_EXPIRED_GRACE`: grace period for removing expired certmagic entries (default `125h`).
When an ACME certificate cannot be obtained, cuesix will use the SSL plugin fallback certificate to keep the `ssls` entry valid. Certmagic keeps retrying, and when a certificate becomes available cuesix triggers a new compile/reload cycle (once a valid config has been delivered before).

## Usage

Standalone:

```bash
cuesix compile --input ./configs --input ./more-configs
```

Server mode:

```bash
cuesix serve \
  --listen :8080 \
  --metrics :9090 \
  --input ./configs \
  --apisix-home /usr/local/apisix \
  --apisix-url http://127.0.0.1:9180
```

## Build

```bash
docker build -t cuesix:latest .
```

## Layout

See `AGENTS.md` for module responsibilities and documentation requirements.
