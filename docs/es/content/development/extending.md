---
title: Extender MailVoid
weight: 40
---

# Extender MailVoid

El código está organizado para que las extensiones comunes toquen un número
reducido de archivos.

## Añadir un tipo de evento

1. Añade una constante en `internal/events/event.go`.
2. Emítelo con `bus.Emit(events.New(events.TypeX, events.SeverityInfo, payload))`.
3. Añádelo a `eventTypes` en `internal/web/handlers_pages.go` para que aparezca en el
   filtro de la consola.
4. Actualiza [la referencia de eventos](/reference/events/).
5. Añade una prueba que compruebe que el evento se emite.

Mantén estables las claves del payload: forman parte de la API pública y del
contrato de auditoría.

## Añadir un campo de política

1. Añade una constante en `internal/models/models.go`.
2. Añádela a `PolicyFields` para que la validación lo acepte y el select de la UI
   lo renderice.
3. Añade un caso a `policy.FieldValue` en `internal/policy/engine.go`.
4. Si pertenece a la concatenación `any`, añádelo ahí también.
5. Añade pruebas para la coincidencia y la validación.
6. Actualiza la guía de políticas y la referencia de la API.

## Añadir una columna de mensaje

1. Añade el campo a `models.Message` y su etiqueta JSON.
2. Añádelo a `dbMessage` en `internal/storage/genji.go`.
3. Añade la columna a la sentencia `CREATE TABLE` y a `messageColumns`.
4. Inclúyelo en `messageToDB` y en `dbMessage.model()`.
5. Actualiza las sentencias INSERT y UPDATE y las listas de argumentos.
6. Actualiza el clon del almacén en memoria si el campo es de tipo puntero o
   referencia.
7. Considera `search_text` si el campo debe ser buscable.
8. Actualiza el resumen de la API si pertenece a las respuestas de listado.
9. Añade pruebas de ida y vuelta del almacenamiento.
10. Haz copia de seguridad y migra las bases de datos existentes; no hay migración
    automática.

## Añadir un método al almacén

1. Añade el método a `storage.Store`.
2. Impleméntalo en `storage.Memory`.
3. Impleméntalo en `storage.Genji` dentro del subconjunto SQL de Genji.
4. Añade aserciones a la suite compartida en `store_test.go`; se ejecuta contra
   ambas implementaciones.
5. Comprueba la concurrencia: las lecturas usan `RLock`, las escrituras usan `Lock`.
6. Evita `ORDER BY` de varias columnas; ordena las claves secundarias en Go.

## Añadir una opción de configuración

1. Añade un campo a `config.Config`.
2. Establece un valor por defecto en `Default()` o `finalize()`.
3. Lee `MAILVOID_*` en `applyEnv`.
4. Añade una flag en `applyFlags`.
5. Valídala si son posibles valores inválidos.
6. Añade una prueba de configuración.
7. Documéntala en [Configuración](/getting-started/configuration/) y
   [CLI y flags](/reference/cli/).

## Añadir un endpoint de API

1. Registra la ruta en `internal/web/server.go`.
2. Implementa el handler en `handlers_api.go`.
3. Analiza la entrada con `BodyParser` de Fiber o con los helpers de query.
4. Devuelve errores mediante `apiError`.
5. Emite un evento si la operación cambia el estado.
6. Añade una prueba usando el helper de cookies en `server_test.go`.
7. Documenta la petición y la respuesta en la referencia de la API.

## Añadir una página o fragmento

1. Crea la plantilla bajo `templates/pages` o `templates/fragments`.
2. Si es una página, añádela a `pageLayouts` en `render.go` y añade una ruta y un
   handler.
3. Usa `s.render` para páginas y `s.renderFragment` para respuestas HTMX.
4. Añade el elemento de navegación a `partials.html` y actualiza `activeFor`/`titleFor`.
5. Mantén el contenido del usuario escapado; usa un iframe con sandbox para el HTML
   del correo.

## Añadir un destino de eventos

Implementa `events.Sink`:

```go
type Sink interface {
    Write(Event) error
}
```

Pásalo a `events.NewBus(...)` en `cmd/mailvoid/main.go`. Los destinos se invocan
de forma síncrona; mantenlos rápidos o almacena en búfer internamente. Un destino
bloqueante ralentiza SMTP.

## Sustituir la base de datos

Implementa `storage.Store` y conéctalo en `main.go`. Nada por encima de `storage`
importa tipos de Genji. Ejecuta la suite de repositorio compartida. Consulta
[Almacenamiento](/reference/storage/).

## Migraciones de esquema

La aplicación crea las tablas con `CREATE TABLE IF NOT EXISTS` y no incluye un
framework de migraciones. Si añades columnas:

- escribe una migración explícita para las bases de datos existentes (o documenta
  una vía de recrear e importar);
- haz copia de seguridad primero;
- verifica con pruebas tanto las bases de datos nuevas como las actualizadas.

## Convenciones de código

- Sin comentarios que repitan el código.
- Documenta los contratos de paquete y los identificadores exportados.
- Mantén la persistencia detrás de `storage.Store`.
- Emite eventos para los cambios de estado.
- Prefiere la biblioteca estándar junto con las dependencias fijadas; no añadas un
  servicio de ejecución externo.
- Añade pruebas en el paquete que modifiques; usa la suite compartida del almacén
  para los cambios de almacenamiento.
