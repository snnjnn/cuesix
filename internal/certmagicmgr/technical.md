Certmagic manager - Technical

Entradas y configuracion:

- `Config`:
  - `Providers`: lista de proveedores ACME. Cada proveedor requiere `name`, `ca` y `email`.
  - `DefaultProvider`: proveedor por defecto cuando no se especifica uno.
  - `DataDir`: ruta de almacenamiento persistente para certmagic.
  - `DefaultTimeout`: timeout por defecto para obtencion de certificados.
- `ProviderConfig`:
  - `Timeout`: timeout especifico para ese proveedor.

Salidas:

- `RequestCertificate` inicia la obtencion asincrona del certificado.

Dependencias:

- `github.com/caddyserver/certmagic`

API principal:

- `NewManager(cfg, logger, events)` valida configuracion y crea una instancia por proveedor. `events` es un canal externo opcional para eventos de certificados.
- `NewWatcher(manager, events)` crea un watcher con un canal de eventos externo (requerido).
- `ChallengeHandler(logger)` devuelve el handler HTTP-01 para `/.well-known/acme-challenge`.
- `RequestCertificate(ctx, provider, sni)`:
  - Inicia la obtencion asincrona del certificado.
  - Usa timeout por proveedor o el timeout global.
  - Serializa el acceso a certmagic con un mutex.
- `RemoveManaged(logger, provider, sni)` elimina la entrada del inventario (no elimina almacenamiento en disco).
- `ClearTracking()` reinicia el mapa de seguimiento de SNIs y elimina el tracking gestionado en certmagic para evitar renovaciones innecesarias.
- `ProviderView.BestMatchFor(sni)` devuelve el mejor certificado conocido para el SNI (el de mayor expiracion).
- `LoadFallbackCertificate(certPath, keyPath)` carga un certificado placeholder para el plugin SSL.

Notas:

- El inventario se actualiza cuando una solicitud se completa con exito.
- `acme://<provider>` selecciona el proveedor en la capa de plugin.
- External Account Binding (EAB) no esta soportado en esta iteracion.
- El handler HTTP-01 se expone por el servidor configurado en el ejecutable y se informa a certmagic con `AltHTTPPort`.
