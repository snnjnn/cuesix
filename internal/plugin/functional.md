# Plugin - Functional

- Allow post-processing of the config at two points:
  - PreRender: merged object before JSON serialization.
  - PostRender: after JSON serialization.
- Plugins run sequentially and stop on the first error.

ssl plugin: Su objetivo es escanear todos los objetos de la lista de `ssls` en la configuración, en busca de atributos `cert`, `key`, `certs` o `keys` que hagan referencia a ficheros o ACME. En el caso de encontrarlos, debe reemplazar el valor de esos atributos por el contenido del fichero o por un certificado obtenido con certmagic, usando los directorios y gestores configurados para el plugin. Si la entrada es malformada (tipos incorrectos o longitudes distintas), no se modifica.

jq plugin: Su objetivo es aplicar transformaciones jq al JSON generado. Busca una entrada de primer nivel `jq` con una lista de objetos que describen las expresiones, elimina esa entrada y aplica las expresiones en cascada por orden de `prio` descendente mediante un pipeline de jq.

yaml plugin: Plugin post-render opcional que recibe el JSON generado, lo convierte a YAML y añade el comentario "#END" al final. Si está habilitado, debe ejecutarse siempre como el último post-render plugin.
