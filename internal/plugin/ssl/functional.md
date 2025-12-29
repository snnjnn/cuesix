El plugin SSL inyecta certificados desde ficheros o los obtiene automáticamente mediante ACME/certmagic.
La deteción de inyección de certificado se basa en el atributo `cert` del objeto `ssls`. Soporta tres formatos:

- `file://<nombre de fichero>`: Un fichero a encontrar dentro de una lista de fs.FS dados.
- `acme://...`: Se resuelve con certmagic.
- cualquier otro formato: Se considera un certificado literal, no se toca.

## Tipo `file`

Las referencias de tipo `file://` se buscan en una lista de directorios dada, en formato de objetos `fs.FS`. Cuando se encuentra el fichero cuyo nombre coincide con la URI, se lee y se reemplaza el valor de `cert` por el contenido del fichero.

Las referencias de tipo `file://` se soportan tanto en `cert` como en `key`.

Al leer el certificado, se intenta obtener su fecha de caducidad. Si se consigue, se almacena la fecha de caducidad por cada SNI (solo se usan los `snis`, que es lo que usa APISIX para el match).

## Tipo `acme`

Las referencias de tipo `acme` se resuelven mediante certmagic. El valor `acme://<provider>` indica el proveedor configurado en el gestor de certmagic.
Para `acme`, la entrada debe incluir exactamente un `sni`. El atributo `key` se ignora y se sobreescribe con la clave obtenida.
La resolución de ACME bloquea durante la compilación hasta obtener el certificado o agotar el timeout configurado.

## Gestión de caducidades

La lista de caducidades es gestionada por una gorutina. El plugin envía los detalles de los certificados que lee, a la gorutrina. La gorotina se encarga de deduplicarlos (una sola entrada por SNI, con la fecha de caducidad más reciente).

Cada `checkInterval` (por defecto, 24 horas), la gorutina decide si hay certificados próximos a caducar. Si hay certificados próximos a caducar hoy (por ejemplo, que no estén caducados aún, pero les queden menos de `warningWindow`, por defecto 5 días), la gorutina envía una notificación para que se recargue la configuración.

Esa notificación utiliza el mismo canal que utiliza el módulo de dispatcher. A efectos funcionales, es indistinguible de una notificación de recarga que llegase por la API de `/compile`, y dispara el mismo proceso. Esto se hace como máximo una vez al día.

La gorutina no se encarga de renovar los certificados; eso lo hace certmagic, o el operador que deja los certificados en el fichero. La gorutina simplemente se encarga de notificar cuando hay certificados próximos a caducar, para que se dispare una reconfiguración.

La reconfiguración tendrá el efecto de volver a ejecutar todos los plugins, entre ellos el de ssl, y con ello actualizar la lista de caducidades. De esta forma, los certificados que el usuario haya actualizado en disco por otros medios, serán recargados.

Si falla el parseo de un certificado para obtener su fecha de caducidad, el error se registra en logs, pero no aborta la compilación.
