El plugin SSL inyecta certificados desde ficheros o los obtiene automáticamente mediante ACME/certmagic.
La deteción de inyección de certificado se basa en el atributo `cert` del objeto `ssls`. Soporta tres formatos:

- `file://<nombre de fichero>`: Un fichero a encontrar dentro de una lista de fs.FS dados.
- `acme://...`: Se resuelve con certmagic.
- cualquier otro formato: Se considera un certificado literal, no se toca.

## Tipo `file`

Las referencias de tipo `file://` se buscan en una lista de directorios dada, en formato de objetos `fs.FS`. Cuando se encuentra el fichero cuyo nombre coincide con la URI, se lee y se reemplaza el valor de `cert` por el contenido del fichero.

Las referencias de tipo `file://` se soportan tanto en `cert` como en `key`.

## Tipo `acme`

Las referencias de tipo `acme` se resuelven mediante certmagic. El valor `acme://<provider>` indica el proveedor configurado en el gestor de certmagic.
Para `acme`, la entrada debe incluir exactamente un `sni`. El atributo `key` se ignora y se sobreescribe con la clave obtenida.
La resolución de ACME bloquea durante la compilación hasta obtener el certificado o agotar el timeout configurado.
Si la obtención del certificado falla, el plugin utiliza un certificado de fallback (placeholder) cargado por el gestor de certmagic, para evitar dejar entradas `ssls` incompletas.
