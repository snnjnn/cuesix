Certmagic manager - Technical

Entradas y configuracion:

- `Config`:
  - `Providers`: lista de proveedores ACME. Cada proveedor requiere `name`, `ca` y `email`.
  - `DefaultProvider`: proveedor por defecto cuando no se especifica uno.
  - `DataDir`: ruta de almacenamiento persistente para certmagic.
  - `ChallengeAddr`: direccion donde se expone el handler HTTP-01.
  - `DefaultTimeout`: timeout por defecto para obtencion de certificados.
  - `FallbackCertPath`: ruta del certificado placeholder de APISIX.
  - `FallbackKeyPath`: ruta de la clave placeholder de APISIX.
- `ProviderConfig`:
  - `Timeout`: timeout especifico para ese proveedor.

Salidas:

- `RequestCertificate` inicia la obtencion asincrona del certificado.
 - El certificado fallback se carga al crear el manager y debe existir; si no, es error fatal.

Dependencias:

- `github.com/caddyserver/certmagic`

API principal:

- `NewManager(cfg, logger, events)` valida configuracion y crea una instancia por proveedor. `events` es un canal externo opcional para eventos de certificados.
- `NewWatcher(manager, events)` crea un watcher con un canal de eventos externo (requerido).
- `RunChallengeServer(ctx)` expone `/.well-known/acme-challenge` en `ChallengeAddr`.
- `RequestCertificate(ctx, provider, sni)`:
  - Inicia la obtencion asincrona del certificado.
  - Usa timeout por proveedor o el timeout global.
  - Serializa el acceso a certmagic con un mutex.
- `RemoveManaged(sni)` elimina la entrada del inventario (no elimina almacenamiento en disco).
 - `Fallback()` (o equivalente) expone el certificado placeholder para que el plugin pueda usarlo en caso de error.
- `ClearTracking()` reinicia el mapa de seguimiento de SNIs, pensado para limpiarlo antes de procesar una nueva configuracion.
- `ProviderView.BestMatchFor(sni)` devuelve el mejor certificado conocido para el SNI (el de mayor expiracion).

Notas:

- El inventario se actualiza cuando una solicitud se completa con exito.
- `acme://<provider>` seleccionara el proveedor en la capa de plugin (no implementado aqui).
- External Account Binding (EAB) no esta soportado en esta iteracion.
- El handler HTTP-01 se expone en `ChallengeAddr` y se informa a certmagic con `AltHTTPPort`.
- Reintentos:
  - Los fallos de obtencion se encolan en un retrier.
  - Tras una recarga completa exitosa, el retrier intenta obtener certificados y un publicador actualiza APISIX via Admin API.
  - Si se inicia un nuevo ciclo de compilacion/recarga, los reintentos se cancelan y el estado se reinicia.
