SSL plugin - Technical

Entradas:

- `Filesystems`: lista de directorios `fs.FS` donde buscar referencias `file://`.
- `ACME`: gestor ACME opcional para referencias `acme://`.
- `Fallback`: certificado y clave PEM para fallback.

Comportamiento:

- Si `cert`/`key` existen y son strings, el plugin intenta resolverlos.
- Si `certs`/`keys` existen, deben ser listas de igual longitud con strings; si no, se dejan sin modificar.
- `acme://<provider>` requiere exactamente un `sni` en la entrada; el `key` se ignora.
- Las referencias `file://` se sustituyen por el contenido del fichero; si no se encuentra, se usa fallback.
- Si no existe gestor ACME o la solicitud falla (I/O, ACME, timeout), se usa fallback.
- Entradas mal formadas (tipos incorrectos, longitudes distintas) se dejan sin tocar.

Salidas:

- Los campos `cert`/`key` o `certs`/`keys` quedan siempre como PEM válidos si la entrada era válida, o como fallback si hubo error.
