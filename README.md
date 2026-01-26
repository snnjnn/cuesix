# sixpack

`sixpack` compila la configuracion standalone de Apache APISIX a partir de fragmentos YAML, aplica reglas de fusion conscientes de APISIX y puede validar/recargar APISIX.

## Configuracion standalone de APISIX en pocas palabras

APISIX tiene un directorio home (por defecto `/usr/local/apisix`), donde vive `conf/`. El modo standalone usa dos archivos de configuracion distintos dentro de la carpeta `conf/`:

- `config.yaml`: config estatica de runtime (puertos, rol, ajustes de admin, etc).
- `apisix.yaml` o `apisix.json`: config dinamica (rutas, servicios, consumidores, plugins).

APISIX determina si la config dinamica es YAML o JSON via `config.yaml`:

```yaml
deployment:
  role: data_plane
  role_data_plane:
    # config provider defines if thee dynamic config
    # is json or yaml. By default, it is yaml.
    config_provider: json|yaml
```

APISIX tambien soporta perfiles via la variable de entorno `APISIX_PROFILE`. Cuando esta definida, APISIX carga `config-<profile>.yaml>` y `apisix-<profile>.[yaml|json]`, en lugar de `comfig.yaml` y `apisix.[yaml|json]`.

## Que hace sixpack

Sixpack construye un archivo de config dinamica unificado, `apisix.[yaml|json]`, leyendo muchos fragmentos de configuracion YAML desde una lista de carpetas de entrada.

Los archivos de configuracion para apisix se ven asi:

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

Sixpack puede fusionar muchos de estos archivos, aplicando un conjunto de **reglas de fusion especificas de apisix**:

- La mayoria de las listas de apisix (como `ssls` o `routes`) se pueden fusionar por una clave como **id**: las listas que contienen objetos con ids diferentes se pueden concatenar.
- Algunas listas, como `consumers`, tienen una `key` de fusion distinta (`name` en lugar de `id`).
- Tipicamente, dos listas no se pueden fusionar si ambas contienen un elemento con la misma clave (`id`, `user`, lo que sea).
- Sin embargo, algunas otras listas **si** se pueden fusionar. Por ejemplo, las listas de `consumers` que contienen el mismo `consumer` se pueden fusionar si el `consumer` en ambas listas solo difiere en el atributo `credentials`. El atributo `credentials` del consumer es una sublista que a su vez se fusiona.

El conjunto completo de reglas de fusion para listas se mantiene en el archivo [compiler.go](internal/compiler/compiler.go).

Por defecto, sixpack generara un `apisix.json` agregado (o `apisix-${APISIX_PROFILE}.json`). No respetara el valor de `deployment.role_data_place.config_provider`. Si necesitas que la salida sea yaml, usa el flag `--plugin-yaml`.

## Funcionalidades

Ademas de fusionar archivos, sixpack implementa algunas funcionalidades de calidad de vida que amplian la expresividad de los archivos yaml de entrada.

### Inlining de certificados

El flag `--plugin-ssl-path` (repetible) activa el plugin SSL. Este plugin escanea entradas de `ssls` para valores `$secret://file/...` o `$secret://acme/...` tanto en `cert`/`key` como en `certs`/`keys`.

- Si una URL de certificado o clave es `$secret://file/...`, busca el nombre de archivo dado en las carpetas especificadas con el flag `--plugin-ssl-path`, y los incrusta en el yaml. Los archivos faltantes se reemplazan con el certificado/clave fallback configurado via `--plugin-ssl-fallback-cert`/`--plugin-ssl-fallback-key`.

Por ejemplo, un snippet de config como:

```yaml
ssls:
  - cert: "$secret://file/tls-domain-name.pem"
```

Se reemplazara por:

```yaml
ssls:
  - cert:
      -----BEGIN CERTIFICATE-----
      ...
      -----END CERTIFICATE-----
```

- Si la URL del certificado es `$secret://acme/...`, intentara generar un nuevo certificado ACME.
  - Los certificados ACME y los SANs son un tema complicado, asi que este modo solo funciona cuando la entrada `ssls` tiene un unico `sni`.
  - La entrada `key` se ignora, se sobrescribe con la key de acme.
  - Si ACME no esta disponible o falla, se usa el certificado/clave fallback.

Si una entrada `ssls` tiene `cert`/`key` o `certs`/`keys` mal formados (tipos incorrectos o longitudes de lista no coinciden), se deja intacta.

### Transformaciones a nivel de configuracion

A veces necesitas post-procesar snippets de `apisix`. Por ejemplo, agregar un plugin particular a todas las rutas que cumplen cierto criterio.

El plugin `jq` te permite incrustar transformaciones basadas en jq dentro de tus snippets de configuracion. Esas transformaciones se aplicaran sobre todo el archivo de config fusionado.

Por ejemplo, si necesitas habilitar basic auth para todas las rutas que comienzan con `/admin/`. Luego de habilitar transformaciones con el flag `--plugin-jq`, puedes usar el siguiente snippet yaml:

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

Las transformaciones se fusionan y se aplican en orden descendente de prioridad (`prio`). El orden dentro de la misma prioridad es indefinido.

Las transformaciones jq siempre deben devolver el objeto de configuracion completo, no pueden ser parciales.

### Proveedor YAML

Por defecto, apisix produce salida json. Puedes usar el flag `--plugin-yaml` para que produzca yaml en su lugar.

Apisix requiere que `apisix.yaml` termine con el comentario `#END`. El plugin yaml de sixpack se encarga de esto.

### Validacion

Cuando se ejecuta en modo servidor, sixpack valida los archivos producidos antes de reemplazar el archivo de config de apisix.

La validacion usa el comando `apisix test` y es automatica. Funciona para archivos json y yaml, y no requiere flags adicionales.

Cuando la validacion falla, se loguea un mensaje de error. Si no falla, el archivo de config se reemplaza.

Para habilitar la validacion de esquema de APISIX, debes proporcionar la URL del endpoint de control de apisix con el flag `--apisix-control-url http://127.0.0.1:9090`.

### Endpoint de esquema

En modo servidor, el servidor de metricas tambien expone `GET /schema`. Devuelve un JSON Schema completo para configuraciones standalone de APISIX, sintetizado a partir del esquema en vivo del control API y el mapping de nivel superior del standalone.

Los detalles de implementacion estan en `internal/schema/README.md`.

### CLI de esquema y fixtures

La CLI `cmd/schema` descarga el esquema en vivo de APISIX e imprime el esquema normalizado a stdout. Por defecto usa la variante estricta; usa `--loose` para mantener las reglas permisivas de IDs de APISIX.

```bash
go run ./cmd/schema --url http://127.0.0.1:9090 > internal/schema/processed_schema.json
go run ./cmd/schema --url http://127.0.0.1:9090 --loose > internal/schema/loose_processed_schema.json
```

Para refrescar el fixture del esquema crudo usado por los tests:

```bash
curl -s http://127.0.0.1:9090/v1/schema > internal/schema/apisix_schema.json
```

Los tests de `internal/schema` comparan el esquema estricto procesado contra `internal/schema/processed_schema.json` y el esquema laxo contra `internal/schema/loose_processed_schema.json`.

## Modos de ejecucion

Standalone (por defecto): compila e imprime la config fusionada a stdout. Opcionalmente puede usar validacion de esquema si se proporciona `--apisix-use-schema` y `--apisix-control-url` apunta a la URL de control de una instancia de apisix en ejecucion. No hace validacion post-build (`apisix test`).

Modo servidor (`sixpack serve`): expone `POST /compile`, `GET /live` y `GET /ready`, ejecuta el pipeline, valida el resultado y escribe el archivo de config en caso de exito. `/ready` devuelve 200 solo despues de que se haya escrito una configuracion exitosa al menos una vez. Certmagic gestiona automaticamente su propio servidor HTTP para desafios ACME en el puerto configurado.

## Flags y variables de entorno

Todos los flags se pueden proporcionar como variables de entorno.

Entrada y modo de ejecucion:
- `--listen` / `SIXPACK_LISTEN`: direccion de escucha para modo servidor (por defecto `127.0.0.1:8080`).
- `--metrics` / `SIXPACK_METRICS_LISTEN`: direccion de escucha para `/metrics` (vacio lo deshabilita).
- `--server-read-header-timeout` / `SIXPACK_SERVER_READ_HEADER_TIMEOUT`: timeout de lectura de headers HTTP del servidor (por defecto `5s`).
- `--server-read-timeout` / `SIXPACK_SERVER_READ_TIMEOUT`: timeout de lectura HTTP del servidor (por defecto `10s`).
- `--server-write-timeout` / `SIXPACK_SERVER_WRITE_TIMEOUT`: timeout de escritura HTTP del servidor (por defecto `10s`).
- `--server-idle-timeout` / `SIXPACK_SERVER_IDLE_TIMEOUT`: timeout de idle HTTP del servidor (por defecto `60s`).
- `--server-shutdown-timeout` / `SIXPACK_SERVER_SHUTDOWN_TIMEOUT`: timeout de apagado del servidor HTTP (por defecto `10s`).
- `--input` (repetible) / `SIXPACK_INPUT_DIRS` (separado por comas): directorios de entrada con fragmentos YAML.
- `--cooldown` / `SIXPACK_COOLDOWN`: demora minima entre ejecuciones de compilacion encoladas.
- `--dry-run` / `SIXPACK_DRY_RUN` (bool): ejecuta el pipeline sin escribir la config.

Rutas de APISIX y validacion:
- `--apisix-home` / `SIXPACK_APISIX_HOME`: directorio home de APISIX (por defecto `/usr/local/apisix`).
- `--mirror-dir` / `SIXPACK_MIRROR_DIR`: directorio mirror opcional para validacion; si esta vacio, sixpack crea un mirror temporal.
- `--keep-mirror` / `SIXPACK_KEEP_MIRROR`: no limpia ni repuebla la carpeta mirror al iniciar.
- `--validation-timeout` / `SIXPACK_VALIDATION_TIMEOUT`: timeout para la validacion `apisix test`.
- `--apisix-use-schema` / `SIXPACK_APISIX_USE_SCHEMA`: valida snippets de config contra el esquema en vivo de APISIX (requiere `--apisix-control-url`).

APISIX Control API:
- `--apisix-control-url` / `SIXPACK_APISIX_CONTROL_URL`: URL base de la Control API de APISIX (por defecto `http://127.0.0.1:9090`).
- `--apisix-api-key` / `SIXPACK_APISIX_API_KEY`: API key de Control API para solicitudes de esquema.
- `--apisix-api-timeout` / `SIXPACK_APISIX_API_TIMEOUT`: timeout para solicitudes HTTP a la Control API.
- `--retry-max` / `SIXPACK_RETRY_MAX`: numero de reintentos de API en falla.
- `--retry-initial` / `SIXPACK_RETRY_INITIAL`: backoff inicial antes del primer reintento.
- `--retry-max-delay` / `SIXPACK_RETRY_MAX_DELAY`: tope para el backoff de reintentos.
- `--retry-multiplier` / `SIXPACK_RETRY_MULTIPLIER`: multiplicador de backoff entre reintentos.

Plugins:
- `--plugin-ssl` / `SIXPACK_PLUGIN_SSL`: habilita el plugin SSL de pre-render (requerido para procesar `$secret://acme/` sin certmagic).
- `--plugin-jq` / `SIXPACK_PLUGIN_JQ`: habilita el plugin jq de post-render.
- `--plugin-jq-timeout` / `SIXPACK_PLUGIN_JQ_TIMEOUT`: timeout para transformaciones jq.
- `--plugin-ssl-path` (repetible) / `SIXPACK_PLUGIN_SSL_PATHS` (separado por comas): rutas de busqueda para archivos de certificados SSL.
- `--plugin-ssl-acme-timeout` / `SIXPACK_PLUGIN_SSL_ACME_TIMEOUT`: timeout para requests ACME del plugin ssl (por defecto `10s`, debe ser positivo).
- `--plugin-ssl-fallback-cert` / `SIXPACK_PLUGIN_SSL_FALLBACK_CERT`: ruta del certificado fallback del plugin ssl (por defecto `${APISIX_HOME}/conf/cert/ssl_PLACE_HOLDER.crt`).
- `--plugin-ssl-fallback-key` / `SIXPACK_PLUGIN_SSL_FALLBACK_KEY`: ruta de la key fallback del plugin ssl (por defecto `${APISIX_HOME}/conf/cert/ssl_PLACE_HOLDER.key`).
- `--plugin-env` / `SIXPACK_PLUGIN_ENV`: nombre del archivo env por directorio usado para sustituciones `${{ VAR }}` de APISIX en snippets de entrada.
- `--plugin-yaml` / `SIXPACK_PLUGIN_YAML`: habilita el plugin YAML de post-render (usar cuando `config_provider: yaml`).

Certmagic (solo serve):
- `--certmagic` / `SIXPACK_CERTMAGIC` (bool): habilita el manager ACME certmagic.
- `--certmagic-provider` (repetible) / `SIXPACK_CERTMAGIC_PROVIDERS` (separado por comas): specs de proveedor (`name|email|ca`).
- `--certmagic-default-provider` / `SIXPACK_CERTMAGIC_DEFAULT_PROVIDER`: nombre del proveedor por defecto.
- `--certmagic-data-dir` / `SIXPACK_CERTMAGIC_DATA_DIR`: directorio de datos de certmagic (requerido cuando esta habilitado).
- `--certmagic-challenge-port` / `SIXPACK_CERTMAGIC_CHALLENGE_PORT`: puerto de desafio HTTP-01 (por defecto `8080`).
- `--certmagic-timeout` / `SIXPACK_CERTMAGIC_TIMEOUT`: timeout por defecto para obtener certificados.
- `--certmagic-watch-interval` / `SIXPACK_CERTMAGIC_WATCH_INTERVAL`: intervalo de refresco para actualizaciones de certificados certmagic (por defecto `1h`).
- `--certmagic-untracked-interval` / `SIXPACK_CERTMAGIC_UNTRACKED_INTERVAL`: intervalo para remover entradas certmagic no rastreadas (por defecto `24h`).
- `--certmagic-untracked-grace` / `SIXPACK_CERTMAGIC_UNTRACKED_GRACE`: periodo de gracia para remover entradas certmagic no rastreadas (por defecto `168h`).
- `--certmagic-cleanup-interval` / `SIXPACK_CERTMAGIC_EXPIRED_INTERVAL`: intervalo para remover entradas certmagic expiradas (por defecto `24h`).
- `--certmagic-expired-grace` / `SIXPACK_CERTMAGIC_EXPIRED_GRACE`: periodo de gracia para remover entradas certmagic expiradas (por defecto `125h`).

Cuando no se puede obtener un certificado ACME, sixpack usara el certificado fallback del plugin SSL para mantener valida la entrada `ssls`. Certmagic sigue reintentando, y cuando un certificado esta disponible sixpack dispara un nuevo ciclo de compilacion.

## Uso

Standalone:

```bash
sixpack compile --input ./configs --input ./more-configs
```

Modo servidor:

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

Ver `AGENTS.md` para responsabilidades de modulos y requisitos de documentacion.
