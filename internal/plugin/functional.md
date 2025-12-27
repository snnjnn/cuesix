# Plugin - Functional

- Allow post-processing of the merged YAML object before serialization.
- Plugins run sequentially and stop on the first error.

El primer plugin implementado es `ssl`. Su objetivo es escanear todos los objetos de la lista de `ssls` en la configuración, en busca de atributos `cert`, `key`, `certs`, `keys` o `client.ca` que hagan referencia a ficheros. En el caso de encontrarlos, debe reemplazar el valor de esos atributos por el contenido del fichero al que hacen referencia usando los directorios configurados para el plugin.
