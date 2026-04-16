# sixpack

`sixpack` compila la configuración standalone de Apache APISIX a partir de fragmentos YAML, aplica reglas de fusión adaptadas a APISIX y puede validar/recargar APISIX.

## Configuración standalone de APISIX en pocas palabras

APISIX tiene un directorio base (por defecto `/usr/local/apisix`), donde se encuentra `conf/`. El modo standalone usa dos archivos de configuración distintos dentro del directorio `conf/`:

- `config.yaml`: config estática de runtime (puertos, rol, ajustes de admin, etc).
- `apisix.yaml` o `apisix.json`: config dinámica (rutas, servicios, consumidores, plugins).

APISIX determina si la config dinámica es YAML o JSON mediante `config.yaml`:

```yaml
deployment:
  role: data_plane
  role_data_plane:
    # config provider defines if thee dynamic config
    # is json or yaml. By default, it is yaml.
    config_provider: json|yaml
```

APISIX también admite perfiles mediante la variable de entorno `APISIX_PROFILE`. Cuando está definida, APISIX carga `config-<profile>.yaml` y `apisix-<profile>.[yaml|json]`, en lugar de `config.yaml` y `apisix.[yaml|json]`.

## Qué hace sixpack

Sixpack construye un archivo de config dinámica unificado, `apisix.[yaml|json]`, leyendo fragmentos de configuración YAML desde una lista de directorios de entrada.

Los archivos de configuración para APISIX tienen este aspecto:

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

Sixpack puede fusionar varios de estos archivos, aplicando un conjunto de **reglas de fusión específicas de APISIX**:

- La mayoría de las listas de APISIX (como `ssls` o `routes`) se pueden fusionar por una clave como **id**: las listas que contienen objetos con IDs distintos se pueden concatenar.
- Algunas listas, como `consumers`, tienen una clave de fusión distinta (`name` en lugar de `id`).
- Normalmente, dos listas no se pueden fusionar si ambas contienen un elemento con la misma clave (`id`, `user`, etc.).
- Sin embargo, algunas listas **sí** se pueden fusionar. Por ejemplo, las listas de `consumers` que contienen el mismo `consumer` se pueden fusionar si ese `consumer` en ambas listas solo difiere en el atributo `credentials`. El atributo `credentials` del `consumer` es una sublista que también se fusiona.

El conjunto completo de reglas de fusión para listas se mantiene en el archivo [compiler.go](internal/compiler/compiler.go).

Por defecto, sixpack generará un `apisix.json` agregado (o `apisix-${APISIX_PROFILE}.json`). No respetará el valor de `deployment.role_data_plane.config_provider`. Si necesitas que la salida sea YAML, usa el flag `--plugin-yaml`.

## Funcionalidades extra

Además de fusionar archivos, sixpack incorpora varias funciones de calidad de vida que amplían la expresividad de los archivos YAML de entrada.

### Inlining de certificados

El flag `--plugin-ssl-path` (repetible) activa el plugin SSL. Este plugin examina entradas de `ssls` en busca de valores `$secret://file/...` o `$secret://acme/...` tanto en `cert`/`key` como en `certs`/`keys`.

- Si una URL de certificado o clave es `$secret://file/...`, busca el nombre de archivo indicado en los directorios definidos con el flag `--plugin-ssl-path`, y lo incrusta en el YAML. Los archivos que falten se sustituyen por el certificado/clave fallback configurado mediante `--plugin-ssl-fallback-cert`/`--plugin-ssl-fallback-key`.

Por ejemplo, un fragmento de config como:

```yaml
ssls:
  - cert: "$secret://file/tls-domain-name.pem"
```

Se reemplazará por:

```yaml
ssls:
  - cert:
      -----BEGIN CERTIFICATE-----
      ...
      -----END CERTIFICATE-----
```

- Si la URL del certificado es `$secret://acme/...`, intentará generar un nuevo certificado ACME.
  - Los certificados ACME y los SAN son un tema complejo, así que este modo solo funciona cuando la entrada `ssls` tiene un único `sni`.
  - La entrada `key` se ignora y se sobrescribe con la clave de ACME.
  - Si ACME no está disponible o falla, se usa el certificado/clave fallback.

Si una entrada `ssls` tiene `cert`/`key` o `certs`/`keys` mal formados (tipos incorrectos o longitudes de lista que no coinciden), se deja intacta.

### Proveedor YAML

Por defecto, APISIX produce salida JSON. Puedes usar el flag `--plugin-yaml` para que genere YAML.

APISIX requiere que `apisix.yaml` termine con el comentario `#END`. El plugin YAML de sixpack se encarga de ello.

### Validación

Cuando se ejecuta en modo servidor, sixpack valida los archivos generados antes de reemplazar el archivo de config de APISIX.

La validación usa el comando `apisix test` y es automática. Funciona para archivos JSON y YAML, y no requiere flags adicionales.

Cuando la validación falla, se registra un mensaje de error. Si no falla, el archivo de config se reemplaza.

Para habilitar la validación de esquema de APISIX, debes proporcionar la URL del endpoint de control de APISIX con el flag `--apisix-control-url http://127.0.0.1:9090`.

## Modos de ejecución

Standalone (`sixpack compile`): compila e imprime la config fusionada en stdout. Opcionalmente puede usar validación de esquema si se proporciona `--apisix-use-schema` y `--apisix-control-url` apunta a la URL de control de una instancia de APISIX en ejecución. No realiza validación post-build (`apisix test`).

Modo servidor (`sixpack serve`): expone `POST /compile`, `GET /live` y `GET /ready`, ejecuta el pipeline, valida el resultado y escribe el archivo de config en caso de éxito. `/ready` devuelve `200` solo después de que se haya escrito una configuración exitosa al menos una vez para un virtual gateway dentro del límite de `--max-gateway-depth` (antes devuelve `425 Too Early`). Certmagic gestiona automáticamente su propio servidor HTTP para desafíos ACME en el puerto configurado.

Modo cliente (`sixpack client`): se conecta por SSE a un `sixpack serve` remoto (`/final/sse/{virtualgw}`) y aplica localmente los payloads recibidos en el `apisix.[json|yaml]` local. También expone `GET /live` y `GET /ready` en su propio `--listen`.

Si activas SSE (`--plugin-sse-keepalive > 0`), el mismo servidor de control también publica:
- `GET /final/full`: redirige (`307`) a `GET /final/full/{virtualgw}` usando el valor configurado en `--apisix-virtual-gateway` (por defecto `default`).
- `GET /final/full/{virtualgw}`: devuelve el último payload aplicado como JSON (`{"at":"...","data":"..."}`), con `ETag`/`Last-Modified`/`Cache-Control`, soporte de `304 Not Modified`, y variante gzip cuando `Accept-Encoding: gzip` (incluye `Vary: Accept-Encoding`). Devuelve `204` si todavía no hay payload aplicado para ese gateway.
- `GET /final/sse`: redirige (`307`) a `GET /final/sse/{virtualgw}` usando el valor configurado en `--apisix-virtual-gateway` (por defecto `default`).
- `GET /final/sse/{virtualgw}`: stream SSE (`text/event-stream`) del gateway virtual indicado que emite el último timestamp conocido y actualizaciones posteriores; además envía comentarios keepalive con el intervalo configurado.

Servidor de métricas/control (por defecto `:8081`): además de `GET /metrics`, publica:
- `GET /schema/sources` y `GET /schema/sources/{path}`
- `GET /schema/virtualgw`, `GET /schema/virtualgw/{virtualgw}` y `GET /schema/virtualgw/{virtualgw}/{kind}/{id}`
- `GET /schema/validate/{path}` (validación de fragmento cacheado con query params como overrides de entorno)
- `POST /schema/validate` (validación de payload inline JSON/YAML con query params como overrides)
- `GET /schema/json`
- `GET /schema/openapi/*`
- `GET /schema/app/*` (UI web de exploración/validación)

## Flags y variables de entorno

Todos los flags se pueden proporcionar como variables de entorno. Esta lista corresponde a los settings definidos en `cmd/sixpack/config`.

Comunes (`compile` y `serve`):
- `--input` (repetible) / `SIXPACK_INPUT_DIRS` (CSV): directorios de entrada con fragmentos YAML (obligatorio).
- `--virtualgw-from-dots` / `SIXPACK_VIRTUALGW_FROM_DOTS` (bool, default false): si el directorio padre del archivo tiene dos o más partes separadas por `.`, se deriva el virtual gateway desde ese nombre (normalizando partes vacías/espacios).
- `--cooldown` / `SIXPACK_COOLDOWN` (default `0s`): retardo mínimo entre ejecuciones de compilación en cola.
- `--apisix-home` / `SIXPACK_APISIX_HOME` (default `/usr/local/apisix`): directorio home de APISIX.
- `--apisix-virtual-gateway` / `SIXPACK_APISIX_VIRTUAL_GATEWAY` (default `default`): Id de gateway virtual por defecto. Una instancia de sixpack puede gestionar configuraciones de varios gateways apisix.
- `--plugin-yaml` / `SIXPACK_PLUGIN_YAML` (default `false`): salida YAML en vez de JSON.
- `--plugin-labels` / `SIXPACK_PLUGIN_LABELS` (default `false`): añade labels de ownership gestionada a los recursos que soportan labels.
- `--mirror-dir` / `SIXPACK_MIRROR_DIR` (default vacío): directorio mirror opcional para validación con `apisix test`.
- `--max-gateway-depth` / `SIXPACK_MAX_GATEWAY_DEPTH` (default `0`): número máximo de separadores `.` permitidos para considerar un virtual gateway en validación y readiness. `0` = solo top-level (`foo`), `1` = hasta un nivel (`foo.bar`), etc. Los gateways con más separadores se compilan y serializan, pero no pasan por validación `apisix test` y no cuentan para `GET /ready`.
- `--keep-mirror` / `SIXPACK_KEEP_MIRROR` (default `false`): no limpiar/poblar de nuevo el mirror al iniciar.
- `--validation-timeout` / `SIXPACK_VALIDATION_TIMEOUT` (default `30s`): timeout para `apisix test`.
- `--apisix-use-schema` / `SIXPACK_APISIX_USE_SCHEMA` (default `false`): valida fragmentos contra esquema APISIX en vivo (requiere `--apisix-control-url`).
- `--dry-run` / `SIXPACK_DRY_RUN` (default `false`): ejecuta pipeline sin escribir config final.
- `--apisix-deployment-mode` / `SIXPACK_APISIX_DEPLOYMENT_MODE` (default `standalone`): modo de operación de APISIX (`standalone`, `traditional`, `decoupled`).
- `--apisix-control-url` / `SIXPACK_APISIX_CONTROL_URL` (default `http://127.0.0.1:9090`): URL base de la Control API.
- `--apisix-admin-url` / `SIXPACK_APISIX_ADMIN_URL` (default `http://127.0.0.1:9091`): URL base de la Admin API (requerida fuera de modo `standalone`).
- `--apisix-api-key` / `SIXPACK_APISIX_API_KEY` (default vacío): API key de la Control API.
- `--apisix-api-timeout` / `SIXPACK_APISIX_API_TIMEOUT` (default `10s`): timeout de requests a la Control API.
- `--retry-max` / `SIXPACK_RETRY_MAX` (default `0`): máximo de reintentos (`<=0` implica backoff sin límite de elapsed).
- `--retry-initial` / `SIXPACK_RETRY_INITIAL` (default `200ms`): backoff inicial.
- `--retry-max-delay` / `SIXPACK_RETRY_MAX_DELAY` (default `2s`): backoff máximo.
- `--retry-multiplier` / `SIXPACK_RETRY_MULTIPLIER` (default `2`): multiplicador de backoff.
- `--retry-jitter` / `SIXPACK_RETRY_JITTER` (default `0.5`): factor de jitter del backoff (`0..1`).
- `--plugin-ssl` / `SIXPACK_PLUGIN_SSL` (default `false`): habilita plugin SSL pre-render.
- `--plugin-ssl-acme-timeout` / `SIXPACK_PLUGIN_SSL_ACME_TIMEOUT` (default `10s`): timeout de requests ACME del plugin SSL.
- `--plugin-ssl-fallback-cert` / `SIXPACK_PLUGIN_SSL_FALLBACK_CERT` (default vacío): ruta cert fallback (si está vacío usa `${APISIX_HOME}/conf/cert/ssl_PLACE_HOLDER.crt` cuando aplica).
- `--plugin-ssl-fallback-key` / `SIXPACK_PLUGIN_SSL_FALLBACK_KEY` (default vacío): ruta key fallback (si está vacío usa `${APISIX_HOME}/conf/cert/ssl_PLACE_HOLDER.key` cuando aplica).
- `--plugin-ssl-path` (repetible) / `SIXPACK_PLUGIN_SSL_PATHS` (CSV): rutas de búsqueda de certificados por archivo.
- `--plugin-env` / `SIXPACK_PLUGIN_ENV` (default vacío): nombre de archivo env por directorio.
- `--certmagic` / `SIXPACK_CERTMAGIC` (default `false`): habilita manager ACME certmagic.
- `--certmagic-default-provider` / `SIXPACK_CERTMAGIC_DEFAULT_PROVIDER` (default vacío): nombre de proveedor certmagic por defecto.
- `--certmagic-data-dir` / `SIXPACK_CERTMAGIC_DATA_DIR` (default vacío): directorio de datos certmagic.
- `--certmagic-challenge-port` / `SIXPACK_CERTMAGIC_CHALLENGE_PORT` (default `8080`): puerto HTTP-01.
- `--certmagic-timeout` / `SIXPACK_CERTMAGIC_TIMEOUT` (default `0s`): timeout para obtener certificados.
- `--certmagic-watch-interval` / `SIXPACK_CERTMAGIC_WATCH_INTERVAL` (default `1h`): intervalo de refresco.
- `--certmagic-untracked-grace` / `SIXPACK_CERTMAGIC_UNTRACKED_GRACE` (default `168h`): gracia para limpiar entradas no rastreadas.
- `--certmagic-expired-interval` / `SIXPACK_CERTMAGIC_EXPIRED_INTERVAL` (default `24h`): intervalo para limpieza de expiradas.
- `--certmagic-expired-grace` / `SIXPACK_CERTMAGIC_EXPIRED_GRACE` (default `125h`): gracia antes de limpiar expiradas.
- `--certmagic-provider` (repetible) / `SIXPACK_CERTMAGIC_PROVIDERS` (CSV): proveedores `name|email|ca`.

Solo `serve`:
- `--listen` / `SIXPACK_LISTEN` (default `127.0.0.1:8080`): dirección de escucha del server de control.
- `--metrics` / `SIXPACK_METRICS_LISTEN` (default `:8081`): dirección de escucha de métricas y `/schema/*` (vacío lo desactiva).
- `--server-read-header-timeout` / `SIXPACK_SERVER_READ_HEADER_TIMEOUT` (default `5s`): timeout de lectura de headers.
- `--server-read-timeout` / `SIXPACK_SERVER_READ_TIMEOUT` (default `10s`): timeout de lectura request.
- `--server-write-timeout` / `SIXPACK_SERVER_WRITE_TIMEOUT` (default `10s`): timeout de escritura response.
- `--server-idle-timeout` / `SIXPACK_SERVER_IDLE_TIMEOUT` (default `60s`): timeout idle keepalive.
- `--server-shutdown-timeout` / `SIXPACK_SERVER_SHUTDOWN_TIMEOUT` (default `10s`): timeout de apagado graceful.
- `--auto-trigger` / `SIXPACK_SERVER_AUTO_TRIGGER` (default `false`): dispara una compilación automática al arrancar.
- `--plugin-sse-keepalive` / `SIXPACK_PLUGIN_SSE_KEEPALIVE` (default `0s`): habilita `/final/full/{virtualgw}`, `/final/sse/{virtualgw}`; el valor define el keepalive del stream.

Solo `client`:
- `--listen` / `SIXPACK_LISTEN` (default `127.0.0.1:8080`): dirección de escucha para `/live` y `/ready` del cliente.
- `--server-read-header-timeout` / `SIXPACK_SERVER_READ_HEADER_TIMEOUT` (default `5s`): timeout de headers del server local del cliente.
- `--server-read-timeout` / `SIXPACK_SERVER_READ_TIMEOUT` (default `10s`): timeout de lectura del server local del cliente.
- `--server-write-timeout` / `SIXPACK_SERVER_WRITE_TIMEOUT` (default `10s`): timeout de escritura del server local del cliente.
- `--server-idle-timeout` / `SIXPACK_SERVER_IDLE_TIMEOUT` (default `60s`): timeout idle del server local del cliente.
- `--server-shutdown-timeout` / `SIXPACK_SERVER_SHUTDOWN_TIMEOUT` (default `10s`): timeout de apagado del server local del cliente.
- `--dry-run` / `SIXPACK_DRY_RUN` (default `false`): consume SSE sin escribir `apisix.[json|yaml]`.
- `--apisix-home` / `SIXPACK_APISIX_HOME` (default `/usr/local/apisix`): home APISIX local para aplicar payloads.
- `--apisix-virtual-gateway` / `SIXPACK_APISIX_VIRTUAL_GATEWAY` (default `default`): gateway virtual que consumirá el cliente (ruta remota `/final/sse/{virtualgw}`) y gateway que puede escribirse en disco local.
- `--plugin-yaml` / `SIXPACK_PLUGIN_YAML` (default `false`): escribe `apisix.yaml` en vez de `apisix.json`.
- `--client-url` / `SIXPACK_CLIENT_URL` (default `http://127.0.0.1:8080`): URL base del sixpack remoto con SSE.
- `--client-connect-timeout` / `SIXPACK_CLIENT_CONNECT_TIMEOUT` (default `5s`): timeout de conexión TCP/TLS y headers iniciales.
- `--client-read-timeout` / `SIXPACK_CLIENT_READ_TIMEOUT` (default `30s`): silencio máximo del stream SSE antes de reconectar.
- `--client-backoff-initial` / `SIXPACK_CLIENT_BACKOFF_INITIAL` (default `1s`): backoff inicial de reconexión.
- `--client-backoff-max-interval` / `SIXPACK_CLIENT_BACKOFF_MAX_INTERVAL` (default `10s`): backoff máximo de reconexión.
- `--client-backoff-multiplier` / `SIXPACK_CLIENT_BACKOFF_MULTIPLIER` (default `2`): multiplicador del backoff de reconexión.
- `--client-backoff-randomization` / `SIXPACK_CLIENT_BACKOFF_RANDOMIZATION` (default `0.5`): jitter del backoff (`0..1`).
- `--client-backoff-max-elapsed` / `SIXPACK_CLIENT_BACKOFF_MAX_ELAPSED` (default `0s`): tiempo máximo total de reintentos (`0` = infinito).
- `--client-backoff-max-retries` / `SIXPACK_CLIENT_BACKOFF_MAX_RETRIES` (default `0`): máximo de reintentos por ciclo (`<=0` = ilimitado).

Cuando no se puede obtener un certificado ACME, sixpack usará el certificado fallback del plugin SSL para mantener válida la entrada `ssls`. Certmagic sigue reintentando y, cuando un certificado está disponible, sixpack lanza un nuevo ciclo de compilación.

## Uso

Standalone:

```bash
sixpack compile --input ./configs --input ./more-configs
```

Modo servidor:

```bash
sixpack serve --listen :8080 --metrics :9090 --input ./configs \
  --apisix-home /usr/local/apisix --plugin-sse-keepalive 10s
```

Modo cliente:

```bash
sixpack client --listen :8080 --apisix-home /usr/local/apisix \
  --client-url http://apisix-server:8080 \
  --apisix-virtual-gateway default
```

## Build

Antes de ejecutar `just build`, instala las dependencias mínimas que la receta asume disponibles:

```bash
# CLI para generar OpenAPI docs
go install github.com/swaggo/swag/cmd/swag@latest

# Asegúrate de que `swag` queda en PATH
export PATH="$(go env GOPATH)/bin:$PATH"

# Dependencias del frontend
npm --prefix internal/app/assets install --ignore-scripts
```

Con eso ya debería funcionar:

```bash
just build
```

Opciones disponibles en el `justfile`:

```bash
# Genera el binario (incluye OpenAPI y frontend)
just build

# Build de imagen Docker con tag custom (incluye OpenAPI y frontend)
just tag sixpack:latest

# Solo genera OpenAPI docs
just swagger

# Solo build del frontend
just app
```

Prerrequisitos según tarea:

- `just`: para ejecutar las recetas.
- `just swagger`: requiere `swag` instalado y disponible en `PATH`.
- `just app`: requiere dependencias de frontend instaladas (`npm --prefix internal/app/assets install --ignore-scripts`).

## Imagen Docker

El `Dockerfile` actual construye una imagen final de APISIX que incluye:
- Módulo dinámico `ngx_http_geoip2_module.so` compilado contra la versión de OpenResty embebida.
- Módulo dinámico `ngx_http_sixpack_buffering_module.so` para poder controlar el buffering de respuestas por ruta.
- Dependencia Lua `lua-resty-maxminddb`.
- Binario `sixpack` en `/usr/local/bin/sixpack`.
