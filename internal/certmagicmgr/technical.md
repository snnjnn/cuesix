Certmagic manager - Technical

Entradas y configuracion:

- `Config`:
  - `Providers`: lista de proveedores ACME. Cada proveedor requiere `name`, `ca` y `email`.
  - `DefaultProvider`: proveedor por defecto cuando no se especifica uno.
  - `DataDir`: ruta de almacenamiento persistente para certmagic.
  - `ChallengeAddr`: direccion donde se expone el handler HTTP-01.
  - `DefaultTimeout`: timeout por defecto para obtencion de certificados.
- `ProviderConfig`:
  - `Timeout`: timeout especifico para ese proveedor.

Salidas:

- `RequestCertificate` devuelve `Certificate` con `CertPEM`, `KeyPEM` y `NotAfter`.
- `ListManaged` devuelve los certificados gestionados con su metadata.

Dependencias:

- `github.com/caddyserver/certmagic`

API principal:

- `NewManager(cfg, logger)` valida configuracion y crea una instancia por proveedor.
- `RunChallengeServer(ctx)` expone `/.well-known/acme-challenge` en `ChallengeAddr`.
- `RequestCertificate(ctx, provider, sni)`:
  - Bloquea hasta obtener o cargar el certificado.
  - Usa timeout por proveedor o el timeout global.
  - Serializa el acceso a certmagic con un mutex.
- `ListManaged()` devuelve el inventario actual.
- `RemoveManaged(sni)` elimina la entrada del inventario (no elimina almacenamiento en disco).

Notas:

- El inventario se actualiza cuando una solicitud se completa con exito.
- `acme://<provider>` seleccionara el proveedor en la capa de plugin (no implementado aqui).
- External Account Binding (EAB) no esta soportado en esta iteracion.
- El handler HTTP-01 se expone en `ChallengeAddr` y se informa a certmagic con `AltHTTPPort`.
