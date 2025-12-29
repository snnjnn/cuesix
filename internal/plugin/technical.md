# Plugin - Technical

- A plugin implements `Update(logger *slog.Logger, value map[string]any) (map[string]any, error)`.
- `Chain` composes multiple plugins and applies them in order, passing the logger through.
- No plugin is required; a nil or empty chain is a no-op.

Plugin `ssl`:

- El plugin SSL debe ser implementado por un objeto que contenga una lista de fs.FS donde pueden encontrarse ficheros de certificados.
- Los atributos `cert`, `key`, `certs`, `keys` o `client.ca` que contienen referencias a ficheros, se identifican porque su valor comienza por `file://`.
- El valor de `file://` únicamente contiene el nombre del fichero al que se hace referencia, no la ruta. El plugin debe buscar el fichero en cualquiera de los fs.FS que contiene.
- La no existencia de un fichero referenciado por un atributo, debe generar un error.
- Si el valor del atributo es `acme://<provider>`, y la entrada tiene un único SNI, el certificado se gestiona automáticamente con certmagic usando el proveedor indicado.
- El plugin se activa desde el ejecutable con `--plugin-ssl-path` o `CUESIX_PLUGIN_SSL_PATHS`.

Plugin `jq`:

- El plugin jq trabaja sobre el JSON renderizado (post-render).
- Debe parsear el JSON con `encoding/json/v2` (Deterministic=true) y buscar una clave de primer nivel `jq`.
- `jq` debe ser una lista de objetos. Cada objeto admite `id` (string, opcional), `prio` (int, opcional) y `expr` (string, obligatorio). Claves desconocidas o tipos incorrectos deben devolver error.
- La entrada `jq` se elimina del resultado antes de aplicar las expresiones.
- Las expresiones se ordenan por `prio` descendente (si falta, se asume 0) y se aplican en cascada construyendo un pipeline de jq.
- La ejecución se realiza enviando el JSON por stdin a jq y leyendo el resultado de stdout, verificando que no existan errores.
- El plugin se activa desde el ejecutable con `--plugin-jq` o `CUESIX_PLUGIN_JQ`.

Plugin `yaml`:

- Plugin post-render opcional que transforma el JSON renderizado en YAML y añade el comentario "#END" al final.
- La conversión a YAML debe parsear el JSON con `encoding/json/v2` (Deterministic=true) y emitir YAML equivalente usando `go.yaml.in/yaml/v4`.
- Si está habilitado, debe ser el último post-render plugin en ejecutarse.
