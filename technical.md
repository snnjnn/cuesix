Tecnologías:

- La aplicación utilizará golang 1.25.
- La aplicación se ejecutará dentro de un contenedor docker.
  - El Dockerfile usará multistage.
  - La primera stage usará la imagen de golang, y construirá la aplicación deshabiitando CGO.
  - La segunda stage será la imagen oficial de apisix, a la que se copiará el ejecutable compilado en la primera stage.
- El uso de cue se hará mediante el módulo cuelang.org/go/cue de golang. Se debe usar la documentación más reciente del módulo, y el MCP de Context7, para adquirir contexto de su API.
- El uso de apisix será por API y línea de comandos.

Generalidades:

- La API http de la aplicación solo está para provocar el disparo de la compilación. No se espera recibir ninguna información en la petición, y la respuesta siempre será un 204 No Content.
- Las rutas a los directorios con los ficheros cue o yaml se proporcionarán como parámetros de la línea de comandos.
- La aplicación debe soportar múltiples directorios de entrada.
- También se le proporcionarán como flags: la ruta al directorio temporal, la ruta al fichero de config de apisix, y la URL de apisix.
- Todos los flags deben poder ser especificados también como variables de entorno.

Desarrollo:

- Usar la librería estándar de golang en lo posible.
- Mantener el mínimo número necesario de dependencias.
- Favorecer mantenibilidad y legibilidad.
