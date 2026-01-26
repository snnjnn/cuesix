# Requisitos funcionales

El objetivo de la aplicación es procesar ficheros de configuración que contienen secciones parciales de la configuración completa de un api gateway apisix, en modo standalone.

Los ficheros de entrada deben estar en formato yaml, como se espera de un fichero de config de apisix estándar. La aplicación usa un algoritmo de unificación específico de APISIX para combinar los fragmentos, considerando que recursos como routes, ssls y upstreams son listas con reglas de fusión distintas según el tipo.

La aplicación trabaja con dos directorios:

1. El directorio de apisix "real", típicamente `/usr/local/apisix`, aunque debe ser configurable.
2. Un mirror temporal, que se crea al inicio en una ruta temporal, y que se usa para validaciones. Esto es debido a limitaciones de `apisix test` (necesita que la config resida en el mismo árbol de directorios que el código, no es capaz de validar una config en un subdirectorio `conf` distinto al que contiene el resto del código de `apisix`)

El fichero combinado generado se almacena en primer lugar en el directorio mirror, y se utiliza `apisix test` para comprobar su validez.

Si el fichero es válido, se reemplaza el fichero de configuración real y se utiliza la API de apisix para provocar su relectura.

La aplicacion tiene dos modos de ejecucion:

- Modo standalone (por defecto): compila los fragmentos y escribe la configuracion resultante en stdout, sin validar ni recargar APISIX.
- Modo servidor (`sixpack serve`): expone endpoints HTTP para disparar la compilacion y, si procede, validar y recargar APISIX.

En modo servidor, la aplicacion expone una ruta "/compile". Cuando se recibe una peticion POST a esa ruta, sin importar el payload o el contenido, se inicia el proceso de recompilacion. Tambien expone endpoints de salud `/live` y `/ready`.

La respuesta a /compile es inmediata, no espera a que se haya ejecutado la compilacion. Internamente la aplicacion hace throttling de las peticiones:

- Si se reciben peticiones /compile nuevas mientras la aplicación está compilando, esas peticiones se encolan, devolviendo inmediatamente la respuesta al cliente.
- Tras cada compilación, hay un periodo de cooldown. El tiempo mínimo entre compilaciones es configurable.

La aplicacion mantiene un hash de la ultima configuracion compilada. El hash se basa en una ordenacion determinista y repetible del fichero generado por el modulo cache. Si el resultado de una compilacion coincide con el anterior, la aplicacion no continua con su proceso.

Cuando el plugin SSL esta activo, se usa un certificado de fallback configurado por flags para resolver referencias `$secret://file/` faltantes o errores ACME.

La aplicacion puede exponer un servidor de metricas y un servidor para desafios ACME si esas direcciones estan configuradas. Certmagic solo se usa en modo servidor; en modo standalone las referencias ACME usan el certificado fallback.

La aplicación se divide en los siguientes componentes:

1. Listener: recibe las llamadas a /compile, y las encola.
2. dispatcher: desencola las llamadas y ejecuta el proceso de compilación. Implementa el throttling.
3. compiler: aplica el algoritmo de unificación de APISIX sobre los ficheros de entrada y genera la salida temporal.
4. cache: valida que la configuración compilada sea diferente a la anterior
5. validator: valida la config dinámica generada en el mirror temporal de `/usr/local/apisix`, y ejecuta `apisix test -c` sobre el `conf/config.yaml` de ese mirror.
6. reloader: reemplaza la config real de apisix y solicita la recarga.
7. plugin: Los plugins realizan preproceso (ejemplo: ssl) y postproceso (jq y yaml) sobre la configuración antes de validarla y cargarla en APISIX.
