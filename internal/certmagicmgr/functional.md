Certmagic manager

Responsabilidades:

- Gestiona certificados ACME con certmagic para dominios solicitados desde el sistema.
- Expone un servidor HTTP independiente para `/.well-known/acme-challenge`.
- Devuelve certificados y claves en PEM para integrarlos en APISIX.
- No soporta External Account Binding (EAB) en esta iteracion.
- Carga un certificado de fallback (placeholder) y lo utiliza cuando falla la obtencion de un certificado ACME.

Comportamiento observable:

- Cada solicitud de certificado bloquea hasta completarse o hasta que expire su timeout.
- Las solicitudes se ejecutan de forma serializada para evitar problemas de reentrancia.
- El servidor de challenge es independiente del servidor de control y del de metricas.
