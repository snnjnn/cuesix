# Requisitos funcionales

El objetivo de la aplicación es procesar ficheros de configuración que contienen secciones parciales de la configuración completa de un api gateway apisix, en modo standalone.

Los ficheros de entrada pueden estar en formato yaml o cuelang. La aplicación usa cue (cuelang versión 0.15 o superior) para unificar todos los ficheros .cue y .yaml en un solo fichero yaml para apisix.

El fichero generado se almacena en primer lugar en un directorio temporal, y se utiliza `apisix test` para comprobar su validez.

Si el fichero es válido, se reemplaza el fichero de configuración real y se utiliza la API de apisix para provocar su relectura.

La aplicación funciona como un servidor web que expone una única ruta, "/compile". Cuando se recibe una petición POST a esa ruta, sin importar el payload o el contenido, se inicia el proceso de recompilación.

La respuesta a /compile es inmediata, no espera a que se haya ejecutado la compilación. Internamente la aplicación hace throttling de las peticiones:

- Si se reciben peticiones /compile nuevas mientras la aplicación está compilando, esas peticiones se encolan, devolviendo inmediatamente la respuesta al cliente.
- Tras cada compilación, hay un periodo de cooldown. El tiempo mínimo entre compilaciones es configurable.

La aplicación mantiene un hash de la última configuración compilada. El hash se basa en una ordenación determinista y repetible del fichero yaml generado. Si el resultado de una compilación coincide con el anterior, la aplicación no continúa con su proceso.

La aplicación se divide en los siguientes componentes:

1. Listener: recibe las llamadas a /compile, y las encola.
2. dispatcher: desencola las llamadas y ejecuta el proceso de compilación. Implementa el throttling.
3. compiler: utiliza `cue` para combinar todos los ficheros de entrada y generar la salida temporal.
4. cache: valida que la configuración compilada sea diferente a la anterior
5. validator: valida la config temporal generada.
6. reloader: reemplaza la config real de apisix y solicita la recarga.