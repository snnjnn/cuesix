# Prueba Técnica Backend Go

## Contexto

La aplicación que se construye en este repo, `sixpack`, compila una configuración única de Apache APISIX a partir de fragmentos YAML con configuraciones parciales. Puedes ver una descripción más amplia del alcance de este proyecto en el [README.md](README.md).

Hoy el proyecto soporta como entrada una lista de rutas en el sistema de ficheros, que escanea para localizar los fragmentos YAML a unificar.

Pero la implementación no está limitada a sistemas de ficheros: En su lugar, utiliza una abstracción `compiler.Input` para modelar cualquier tipo de orígen de datos que sea capaz de producir una secuencia de fragmentos YAML.

En este reto, nos vamos a plantear el desarrollo de un nuevo origen de datos para los fragmentos YAML: Una **base de datos**.

## Objetivo

La prueba está dividida en dos retos complementarios:

- Conexión a base de datos: Reto de implementación.
- Actualización del estado: Reto de diseño.

## Reto 1: Conexión a base de datos

El primer reto es implementar la interfaz `compiler.Input` usando como backend una base de datos. El módulo `internal/inputs/db` define buena parte del scaffolding necesario, y un test que sirve de orientación sobre el resultado esperado.

El objetivo del reto es implementar las funciones del tipo `db.Input` en el fichero [internal/inputs/db/input.go](internal/inputs/db/input.go), para que se alineen con la interfaz `compiler.Input`: 

```go
type Input interface {
	Namespaces() []string
	Enumerate(namespace string) iter.Seq2[SourceRef, error]
	Open(ref SourceRef) (io.ReadCloser, error)
}
```

- `Namespaces()`:
  - Debe devolver la lista de namespaces disponibles en la tabla snippets.
  - La lista debe estar ordenada de forma estable en orden lexicográfico ascendente.
  - No debe contener duplicados.
  - Si no hay snippets, debe devolver una lista vacía.

- `Enumerate(namespace string)`
  - Debe enumerar todos los snippets pertenecientes al namespace indicado.
  - Cada SourceRef debe construirse con: `Namespace = namespace`, `Path = "{virtualgw}/{name}"`.
  - La enumeración debe producir un orden estable y determinista.
  - Si el namespace no existe, debe devolver una secuencia vacía.
  - Si ocurre un error al consultar la base de datos, debe emitirse como error en la secuencia.

- `Open(ref SourceRef)`
  - Debe cargar el contenido del snippet identificado por `ref.Namespace` y `ref.Path`.
  - Debe interpretar `ref.Path` como `{virtualgw}/{name}`.
  - Si la combinación (`namespace`, `virtualgw`, `name`) no existe en la base de datos, debe devolver un error compatible con `fs.ErrNotExist`.
  - Si `ref.Path` no tiene un formato válido, debe devolver un error.
  - El `ReadCloser` devuelto debe permitir leer el contenido completo almacenado en `content`.

La solución debe integrarse con el test ya presente en:

- [internal/inputs/db/input_test.go](internal/inputs/db/input_test.go)

### Alcance

Debes completar:

- [internal/inputs/db/input.go](internal/inputs/db/input.go)

Puedes modificar o ampliar los tests de:

- [internal/inputs/db/input_test.go](internal/inputs/db/input_test.go)

Si lo consideras necesario, puedes añadir más tests en ese paquete.

### Modelo de datos esperado

La implementación debe asumir una tabla `snippets` con este esquema:

```sql
CREATE TABLE snippets (
  namespace TEXT NOT NULL,
  virtualgw TEXT NOT NULL,
  name TEXT NOT NULL,
  content TEXT NOT NULL,
  PRIMARY KEY (namespace, virtualgw, name)
);
```

- Cada fila representa un snippet YAML.
- El atributo `Namespace` del objeto `SourceRef` se corresponde con la columna `namespace` de la tabla.
- El atributo `Path` del objeto `SourceRef` se corresponde con la concatenación de las columnas `{virtualgw}/{name}`.

### Qué valoraremos

- Claridad de la solución.
- Idoneidad de los tests.
- Tratamiento explícito de errores.
- Coherencia con el resto del código.

## Reto 2: Diseño

Este segundo reto es exclusivamente de diseño.

### Antecedentes

La aplicación `sixpack` está pensada para poder funcionar como un servidor que se mantiene en ejecución, y puede re-compilar los snippets YAML bajo demanda.

La idea es que existen procesos externos que ocasionalmente **cambian los snippets que hay en la fuente** (el sistema de ficheros o la base de datos). Estos cambios deben volver a disparar el proceso de compilación y unificación, generando una nueva configuración final.

Para eso,

- `sixpack` lanza un bucle de control que se encarga del proceso completo (lectura de snippets / unificación / validación / actualización de apisix).
- El módulo responsable de gestionar este bucle es `internal/dispatcher`.
- El *dispatcher* lanza una gorutina que se mantiene a la espera de órdenes de ejecución.
- Las ejecuciones se disparan mediante un método `Notify` expuesto por el *dispatcher*, que provoca la ejecución del bucle:

```go
// Notifier is notified when /compile is requested.
type Notifier interface {
	Notify()
}
```

Otros módulos, como `internal/listener`, reciben una referencia a un `Notifier` para poder provocar una nueva ejecución del bucle de control, cuando se detecta un cambio en las fuentes.

Actualmente sólo ese módulo `internal/listener` es capaz de notificar al *dispatcher*, cuando el servidor recibe una petición `HTTP POST` en la ruta `/compile`.
 
### Objetivo del reto

El objetivo de este reto es avanzar en la integración de `sixpack` con base de datos, haciendo que **cualquier cambio en la base de datos** provoque una llamada al método `Notify` del *dispatcher*, para que se vuelva a ejecutar el proceso de compilación.

Existen multitud de bases de datos con enormes diferencias en sus capacidades. Diferentes bases de datos pueden tener diferentes mecanismos para detectar y reaccionar a cambios en las tablas.

En este ejercicio nos interesara sólo considerar las capacidades de estos dos motores de bases de datos:

- sqlite
- postgresql

El candidato debería hacer una breve propuesta de diseño que responda a preguntas como:

- ¿Te ofrece cada base de datos particular algún mecanismo alternativo al polling? ¿Qué ventajas o inconvenientes tiene usarlo? 
- ¿Como plantearías la integración con el *dispatcher*, para que sea agnóstica a la base de datos y mecanismo en uso?
- ¿Qué errores pueden afectar a la robustez de la solución? ¿Cómo tratarías los posibles errores? ¿Cómo los harías visibles para el usuario o para el equipo de desarrollo?

### Entregable

La respuesta que se espera es una propuesta de alto nivel, sólo con interfaces (sin implementaciones concretas), pero orientada a una arquitectura particular.

Es decir, no se espera código concreto. Pero tampoco una disertación teórica sobre diferentes mecanismos de notificación que soporta cada base de datos, con pros y contras.

En una buena propuesta, lo que esperamos es:

- Diseñas una interfaz de alto nivel, agnóstica al mecanismo de sincronización particular de la base de datos. Haces una primera propuesta de los tipos y métodos que expones como API pública del módulo.
- Describes como conecta esa API con la API de *dispatcher*. ¿Quién es responsable de invocar a `Notify`? ¿cómo y cuándo?
- Seleccionas razonadamente alguno de los mecanismos posibles, según el motor de base de datos, para identificar si ha habido cambios en las tablas desde la última notificación.
- Identificas qué cambios tendrías que considerar en el esquema, para adoptar ese mecanismo: ¿necesitarías nuevas tablas, columnas, vistas, triggers...? ¿dónde, y para qué?
- Explicas brevemente cómo implementarías la interfaz que has definido, para cada base de datos. ¿Hay gorutinas? ¿quien y cómo las lanza? ¿en qué condiciones se detienen? ¿dónde puede haber errores? ¿cómo podríamos tratarlos?

## Entregable

Esperamos una propuesta que incluya:

- Reto 1: implementación de `internal/inputs/db`.
- Reto 1: tests automatizados.
- Reto 2: una descripción de la solución de diseño propuesta para soportar recarga ante cambios en base de datos, idealmente no más de dos páginas.

Puedes entregar:

- Un parche o PR sobre este repositorio.
- O un archivo comprimido con los cambios y una nota breve de implementación.

## Cómo ejecutar

El proyecto usa Go y ya incluye dependencias para testear con SQLite (`sqlx` y `modernc.org/sqlite`).

Una forma razonable de validar la solución es ejecutar:

```bash
go test ./...
```

Si prefieres limitarte al paquete de la prueba:

```bash
go test ./internal/inputs/db
```

