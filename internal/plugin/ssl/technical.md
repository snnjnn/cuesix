Gestión de certificados:

- El módulo debe mantener una lista deduplicada de certificados con fecha de caducidad, indexada por SNI (solo se usa `snis`, que es lo que usa APISIX para el match). En principio puede ser un `map[string]time.Time`.
- Por ahora solo se consideran certificados de tipo `file://`. ACME/certmagic se añadirá en una fase posterior.
- La lista de caducidades se actualiza durante la ejecución del plugin SSL. Por cada certificado leído, se intenta parsear la fecha de caducidad y se registra una entrada por cada SNI. Si falla el parseo, se loguea el error y se continúa.
- Periodicamente, el módulo ejecuta un proceso de notificación:
  - Si hay algún certificado no caducado en la lista de certificados en uso, pero que vaya a caducar en menos de `warningWindow` (por defecto 5 días), se lanza una notificación usando la misma API que dispatcher: la interfaz `Notify()`.
  - La periodicidad se controla con `checkInterval` (por defecto 24 horas).
  - La notificación se envía como máximo una vez al día.

- Cada vez que el plugin ssl empieza la gestión de una nueva configuración, se debe borrar la lista de certificados en uso.
- Cada vez que el plugin ssl empieza la gestión de una nueva configuración, se debe inhibir el proceso de notificación del día en curso. Es decir, el día que se procesa una config nueva, no se vuelven a disparar notificaciones por certificados cerca de caducar.
