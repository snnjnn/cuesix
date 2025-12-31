Certmagic manager

Responsabilidades:

- Gestiona certificados ACME con certmagic para dominios solicitados desde el sistema.
- Expone un handler HTTP para `/.well-known/acme-challenge`.
- Expone certificados en PEM mediante el watcher para integrarlos en APISIX.
- No soporta External Account Binding (EAB) en esta iteracion.

Comportamiento observable:

- Cada solicitud de certificado inicia la obtencion en segundo plano y respeta el timeout configurado.
- Las solicitudes se serializan por proveedor para evitar problemas de reentrancia.
- El watcher notifica cambios de certificados y mantiene un cache de seguimiento por SNI/proveedor.
- El proceso puede ejecutar limpiezas periodicas para dejar de seguir entradas no observadas dentro del periodo de gracia.
