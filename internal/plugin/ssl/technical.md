Gestión de certificados:

- El módulo debe mantener una lista deduplicada de certificados con fecha de caducidad, indexada por SNI (solo se usa `snis`, que es lo que usa APISIX para el match). En principio puede ser un `map[string]time.Time`.
- Se consideran certificados de tipo `file://` y `acme://`.
- La lista de caducidades se actualiza durante la ejecución del plugin SSL. Por cada certificado leído, se intenta parsear la fecha de caducidad y se registra una entrada por cada SNI. Si falla el parseo, se loguea el error y se continúa.
  - Para certificados ACME, se registra la caducidad devuelta por certmagic.
- Periodicamente, el módulo ejecuta un proceso de notificación:
  - Si hay algún certificado no caducado en la lista de certificados en uso, pero que vaya a caducar en menos de `warningWindow` (por defecto 5 días), se lanza una notificación usando la misma API que dispatcher: la interfaz `Notify()`.
  - La periodicidad se controla con `checkInterval` (por defecto 24 horas).
  - La notificación se envía como máximo una vez al día.

- Cada vez que el plugin ssl empieza la gestión de una nueva configuración, se debe borrar la lista de certificados en uso.
- Cada vez que el plugin ssl empieza la gestión de una nueva configuración, se debe inhibir el proceso de notificación del día en curso. Es decir, el día que se procesa una config nueva, no se vuelven a disparar notificaciones por certificados cerca de caducar.

Integración ACME:

- Si `cert` tiene el formato `acme://<provider>`, se solicita el certificado al gestor de certmagic.
- La entrada debe incluir exactamente un `sni`. Si no es así, se devuelve error.
- La llamada bloquea hasta obtener el certificado o agotar el timeout configurado.
- El atributo `key` se ignora y se sobreescribe con la clave obtenida.
- Si la obtención del certificado falla, se usa el certificado de fallback cargado por el gestor de certmagic (placeholder de APISIX), y se registra el error.

Reintentos y actualizaciones incrementales:

- Los SNIs que fallan se encolan para reintentos asíncronos.
- Un proceso en segundo plano reintenta la obtención y, cuando se consigue, actualiza la entrada de `ssl` en APISIX mediante la API de administración, sin recargar el resto de la configuración.
- Los reintentos y la publicación incremental se inhiben durante la ejecución del pipeline y se limpian al iniciar un nuevo ciclo de compilación. Solo se activan cuando la recarga completa ha finalizado correctamente.
