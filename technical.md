Tecnologías:

- La aplicación utilizará golang 1.25.
- La aplicación se ejecutará dentro de un contenedor docker.
  - El Dockerfile usará multistage.
  - La primera stage usará la imagen de golang, y construirá la aplicación deshabiitando CGO.
  - La segunda stage será la imagen oficial de apisix, a la que se copiará el ejecutable compilado en la primera stage.
- La unificación de configuraciones se realizará mediante un algoritmo propio de fusión, ajustado a las listas y claves naturales de los recursos de APISIX.
- El uso de apisix será por API y línea de comandos.

Generalidades:

- La API http de la aplicación solo está para provocar el disparo de la compilación. No se espera recibir ninguna información en la petición, y la respuesta siempre será un 204 No Content.
- Las rutas a los directorios con los ficheros yaml se proporcionarán como parámetros de la línea de comandos.
- La aplicación debe soportar múltiples directorios de entrada.
- También se le proporcionarán como flags: la ruta al directorio temporal, la ruta al fichero de config de apisix, y la URL de apisix.
- Todos los flags deben poder ser especificados también como variables de entorno.

Reglas de fusión APISIX:

- El fichero standalone de APISIX usa listas por recurso (plural). Se soportan estas listas y claves:
  - routes: id (opcional)
  - services: id (opcional)
  - upstreams: id (opcional)
  - ssls: id (opcional)
  - global_rules: id (obligatorio)
  - consumer_groups: id (obligatorio)
  - plugin_configs: id (obligatorio)
  - stream_routes: id (opcional)
  - protos: id (opcional)
  - consumers: username (obligatorio)
  - consumers.credentials: credential_id (obligatorio)
  - plugin_metadata: plugin_name (obligatorio)
- Las listas con id opcional no generan ids. Las entradas sin id se mantienen como elementos independientes en el resultado.
- Para cada lista, la fusión por clave funciona asi:
  - Si la clave no se repite, el elemento se copia tal cual.
  - Si la clave se repite, se aplica fusion profunda: los escalares deben ser iguales o estar presentes en un solo lado; los mapas se fusionan recursivamente; las listas solo se fusionan si tienen regla definida.
- Los solapamientos sin regla definida deben producir error.
- Algunos recursos permiten solapamiento si solo difieren en sublistas con regla (ejemplo: consumers pueden fusionarse por username si solo aportan credentials; rutas si solo difieren en plugins configurables).

Configuracion de reglas de fusion (ejemplo conceptual):

- path: /consumers
  kind: list
  id_attr: username
  id_optional: false
  allow_merge_same_id: true
  children:
    /credentials:
      kind: list
      id_attr: credential_id
      id_optional: false
      allow_merge_same_id: false

Desarrollo:

- Usar la librería estándar de golang en lo posible.
- Mantener el mínimo número necesario de dependencias.
- Favorecer mantenibilidad y legibilidad.
