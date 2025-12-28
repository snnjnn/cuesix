# Requisitos funcionales

El objetivo de la aplicación es procesar ficheros de configuración que contienen secciones parciales de la configuración completa de un api gateway apisix, en modo standalone.

Los ficheros de entrada deben estar en formato yaml, como se espera de un fichero de config de apisix estándar. La aplicación usa un algoritmo de unificación específico de APISIX para combinar los fragmentos, considerando que recursos como routes, ssls y upstreams son listas con reglas de fusión distintas según el tipo.

La aplicación trabaja con dos directorios:

1. El directorio de apisix "real", típicamente `/usr/local/apisix`, aunque debe ser configurable.
2. Un mirror temporal, que se crea al inicio en una ruta temporal, y que se usa para validaciones. Esto es debido a limitaciones de `apisix test` (necesita que la config resida en el mismo árbol de directorios que el código, no es capaz de validar una config en un subdirectorio `conf` distinto al que contiene el resto del código de `apisix`)

El fichero combinado generado se almacena en primer lugar en el directorio mirror, y se utiliza `apisix test` para comprobar su validez.

Si el fichero es válido, se reemplaza el fichero de configuración real y se utiliza la API de apisix para provocar su relectura.

La aplicación funciona como un servidor web que expone una única ruta, "/compile". Cuando se recibe una petición POST a esa ruta, sin importar el payload o el contenido, se inicia el proceso de recompilación.

La respuesta a /compile es inmediata, no espera a que se haya ejecutado la compilación. Internamente la aplicación hace throttling de las peticiones:

- Si se reciben peticiones /compile nuevas mientras la aplicación está compilando, esas peticiones se encolan, devolviendo inmediatamente la respuesta al cliente.
- Tras cada compilación, hay un periodo de cooldown. El tiempo mínimo entre compilaciones es configurable.

La aplicación mantiene un hash de la última configuración compilada. El hash se basa en una ordenación determinista y repetible del fichero generado por el modulo cache. Si el resultado de una compilación coincide con el anterior, la aplicación no continúa con su proceso.

La aplicación se divide en los siguientes componentes:

1. Listener: recibe las llamadas a /compile, y las encola.
2. dispatcher: desencola las llamadas y ejecuta el proceso de compilación. Implementa el throttling.
3. compiler: aplica el algoritmo de unificación de APISIX sobre los ficheros de entrada y genera la salida temporal.
4. cache: valida que la configuración compilada sea diferente a la anterior
5. validator: valida la config dinámica generada en el mirror temporal de `/usr/local/apisix`, y ejecuta `apisix test -c` sobre el `conf/config.yaml` de ese mirror.
6. reloader: reemplaza la config real de apisix y solicita la recarga.
7. plugin: Los plugins realizan preproceso (ejemplo: ssl) y postproceso (jq y yaml) sobre la configuración antes de validarla y cargarla en APISIX.
