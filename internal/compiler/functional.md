# Compiler: requisitos funcionales

El modulo compiler transforma varios fragmentos YAML en una sola configuracion standalone de APISIX.

Comportamiento observable:

- Recorre todos los ficheros `.yaml` y `.yml` dentro de los sistemas de ficheros proporcionados.
- Interpreta cada fichero como YAML y lo fusiona con el resultado acumulado.
- Usa reglas de fusion especificas por recurso (listas por clave y sublistas).
- Si encuentra conflictos entre fragmentos incompatibles, devuelve error.
- El resultado es un mapa listo para serializarse.
