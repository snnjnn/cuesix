Certmagic manager

Responsabilidades:

- Gestiona certificados ACME con certmagic para dominios solicitados desde el sistema.
- Expone un servidor HTTP independiente para `/.well-known/acme-challenge`.
- Expone certificados en PEM mediante el watcher para integrarlos en APISIX.
- No soporta External Account Binding (EAB) en esta iteracion.
- Carga un certificado de fallback (placeholder) y lo utiliza cuando falla la obtencion de un certificado ACME.

Comportamiento observable:

- Cada solicitud de certificado inicia la obtencion en segundo plano y respeta el timeout configurado.
- Las solicitudes se serializan por proveedor para evitar problemas de reentrancia.
- El servidor de challenge es independiente del servidor de control y del de metricas.
- El watcher notifica cambios de certificados y mantiene un cache de seguimiento por SNI.
