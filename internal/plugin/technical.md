# Plugin - Technical

- A plugin implements `Update(value map[string]any) (map[string]any, error)`.
- `Chain` composes multiple plugins and applies them in order.
- No plugin is required; a nil or empty chain is a no-op.

Plugin `ssl`:

- El plugin SSL debe ser implementado por un objeto que contenga una lista de fs.FS donde pueden encontrarse ficheros de certificados.
- Los atributos `cert`, `key`, `certs`, `keys` o `client.ca` que contienen referencias a ficheros, se identifican porque su valor comienza por `file://`.
- El valor de `file://` únicamente contiene el nombre del fichero al que se hace referencia, no la ruta. El plugin debe buscar el fichero en cualquiera de los fs.FS que contiene.
- La no existencia de un fichero referenciado por un atributo, debe generar un error.
- El plugin se activa desde el ejecutable con `--plugin-ssl-path` o `CUESIX_PLUGIN_SSL_PATHS`.
