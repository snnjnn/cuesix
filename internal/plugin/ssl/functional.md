El plugin SSL inyecta certificados desde ficheros o los obtiene automáticamente mediante ACME/certmagic.
La detección se realiza sobre los campos `cert`/`key` o las listas `certs`/`keys` del objeto `ssls`. Soporta tres formatos:

- `file://<nombre de fichero>`: Un fichero a encontrar dentro de una lista de fs.FS dados.
- `acme://...`: Se resuelve con certmagic.
- cualquier otro formato: Se considera un certificado literal, no se toca.

Si los campos o listas están mal formados (por ejemplo, tipos no string o longitudes distintas), el plugin no los modifica y asume que el error es del input.

## Tipo `file`

Las referencias de tipo `file://` se buscan en una lista de directorios dada, en formato de objetos `fs.FS`. Cuando se encuentra el fichero cuyo nombre coincide con la URI, se lee y se reemplaza el valor por el contenido del fichero. Si no se encuentra, se usa el certificado de fallback.

Las referencias de tipo `file://` se soportan tanto en `cert` como en `key`, y también en las listas `certs`/`keys`.

## Tipo `acme`

Las referencias de tipo `acme` se resuelven mediante certmagic. El valor `acme://<provider>` indica el proveedor configurado en el gestor de certmagic.
Para `acme`, la entrada debe incluir exactamente un `sni`. El atributo `key` se ignora y se sobreescribe con la clave obtenida.
La resolución de ACME bloquea durante la compilación hasta obtener el certificado o agotar el timeout configurado.
Si la obtención del certificado falla, el plugin utiliza un certificado de fallback (placeholder) configurado en el arranque, para evitar dejar entradas `ssls` incompletas.
