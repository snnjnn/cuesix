Certmagic manager - Technical

Entradas y configuracion:

- `Config`:
  - `Providers`: lista de proveedores ACME. Cada proveedor requiere `name`, `email` y `ca`.
  - `DefaultProvider`: proveedor por defecto cuando no se especifica uno.
  - `DataDir`: ruta de almacenamiento persistente para certmagic.
  - `DefaultTimeout`: timeout por defecto para obtencion de certificados.

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
  - Usa el timeout global configurado.
  - Serializa el acceso a certmagic con un mutex.
- `RemoveUntracked(ctx, logger, gracePeriod)` elimina el tracking gestionado en certmagic para entradas que no se hayan observado dentro del periodo de gracia.
  - Si no se puede resolver el proveedor, no elimina la entrada de seguimiento local.
- `RemoveExpired(ctx, logger, interval, gracePeriod)` limpia certificados expirados en el almacenamiento con el intervalo y periodo de gracia configurados.
- `ClearTracking()` marca una nueva generacion de seguimiento para que `RemoveUntracked` considere candidatas las entradas antiguas.
- `ProviderView.BestMatchFor(sni)` devuelve el mejor certificado conocido para el SNI (el de mayor expiracion).
- `LoadFallbackCertificate(certPath, keyPath)` carga un certificado placeholder para el plugin SSL.

Formato de proveedor:

- `ParseProviderSpec` acepta el formato fijo `name|email|ca`.

Notas:

- El inventario se actualiza cuando una solicitud se completa con exito.
- `acme://<provider>` selecciona el proveedor en la capa de plugin.
- External Account Binding (EAB) no esta soportado en esta iteracion.
- El handler HTTP-01 se expone por el servidor configurado en el ejecutable y se informa a certmagic con `AltHTTPPort`.
