---
title: Almacenamiento
weight: 50
---

# Almacenamiento

## La interfaz del repositorio

Todas las operaciones de persistencia pasan por `internal/storage.Store`:

```go
type Store interface {
    events.Sink

    SaveMessage(ctx context.Context, m *models.Message) error
    GetMessage(ctx context.Context, id string) (*models.Message, error)
    ListMessages(ctx context.Context, f models.MessageFilter) ([]*models.Message, error)
    CountMessages(ctx context.Context, f models.MessageFilter) (int, error)
    DeleteMessage(ctx context.Context, id string) error
    DeleteExpired(ctx context.Context, now time.Time) ([]*models.Message, error)

    SavePolicy(ctx context.Context, p *models.Policy) error
    GetPolicy(ctx context.Context, id string) (*models.Policy, error)
    ListPolicies(ctx context.Context) ([]*models.Policy, error)
    DeletePolicy(ctx context.Context, id string) error
    ReorderPolicies(ctx context.Context, ids []string) error

    SaveEvent(ctx context.Context, ev events.Event) error
    ListEvents(ctx context.Context, f models.EventFilter) ([]events.Event, error)
    CountEvents(ctx context.Context, f models.EventFilter) (int, error)

    GetUser(ctx context.Context, username string) (*models.User, error)
    SaveUser(ctx context.Context, u *models.User) error

    Close() error
}
```

El almacén también es un `events.Sink` (`Write(Event) error`), de modo que el bus
de eventos puede persistir eventos sin saber si el almacén de respaldo es memoria
o Genji.

## Implementaciones

| Implementación | Uso |
|----------------|-----|
| `storage.Genji` | Producción, persistente |
| `storage.Memory` | Pruebas y uso efímero |

`storage.Memory` es totalmente funcional para la captura y la evaluación de
políticas. Lo usa la suite de pruebas y es una opción conveniente si integras
MailVoid en una aplicación más grande que proporciona su propia persistencia.

## Genji

### Versión y justificación

MailVoid fija **Genji v0.15.3**, la última versión publicada bajo
`github.com/genjidb/genji`. Las versiones posteriores declaran la ruta de módulo
`github.com/chaisql/chai`, y v0.17+ requieren Go 1.25. El desarrollo upstream se
trasladó a Chai, pero v0.15.3 se compila sin problemas en Go 1.24 y admite las
formas de consulta que MailVoid necesita.

### Ubicación de la base de datos

`genji.Open(<db-path>)` crea un **directorio** en la ruta configurada. Es un
almacén respaldado por Pebble que contiene:

```text
data/mailvoid.db/
├── 000002.log
├── CURRENT
├── LOCK
├── MANIFEST-000001
└── OPTIONS-000003
```

Un archivo `LOCK` impide que dos procesos abran el mismo directorio. Ejecuta solo
una instancia de MailVoid por directorio de datos.

### Creación del esquema

Al iniciar, el repositorio ejecuta `CREATE TABLE IF NOT EXISTS` para:

- `messages`
- `policies`
- `events`
- `users`

No hay un framework de migración. Añadir columnas a una base de datos existente
requiere una migración deliberada. Haz una copia de seguridad del directorio de
datos antes de actualizar entre versiones que cambian el esquema.

### Concurrencia

- Las escrituras toman un mutex exclusivo.
- Las lecturas toman un RWMutex compartido.
- Las escrituras del sumidero de eventos pasan por el mismo bloqueo.
- El detector de carreras pasa para la suite de pruebas del almacén compartido.

### Notas sobre SQL de Genji

El repositorio se mantiene deliberadamente dentro de un pequeño subconjunto de
SQL:

- `LIKE`, `IN`, `LIMIT`, `OFFSET`, `COUNT(*)` y argumentos posicionales `?`;
- una única expresión `ORDER BY` por consulta (la ordenación secundaria se aplica
  en Go);
- `LOWER()` no está disponible, por lo que la búsqueda sin distinción de
  mayúsculas usa la columna `search_text` preconvertida a minúsculas;
- sin upsert nativo: las inserciones recurren a actualizaciones cuando
  `genji.IsAlreadyExistsError` es verdadero.

## Almacén en memoria

`storage.Memory` mantiene mapas detrás de un RWMutex:

- los mensajes se clonan en lectura y escritura;
- las consultas de listado filtran y ordenan copias fuera del bloqueo;
- los eventos se acumulan en un slice;
- sin persistencia entre reinicios.

## Semántica de filtrado

Ambas implementaciones comparten el mismo comportamiento de filtrado:

- `Query` es una subcadena sobre `search_text` sin distinción de mayúsculas.
- `Domain`, `Mailbox` y `Action` son coincidencias exactas.
- `Port` es exacto.
- `Since` y `Until` son límites inclusivos sobre la hora de recepción.
- `Limit` es 50 por defecto y tiene un máximo de 500 en el repositorio. Los
  manejadores HTTP tienen un máximo de 200.
- `Offset` se basa en cero.

Los listados de mensajes van de más reciente a más antiguo. Los listados de
eventos van de más reciente a más antiguo.

## Copia de seguridad

La base de datos es un directorio y el registro de eventos es un archivo. Detén el
proceso y copia ambos juntos. Consulta
[Copia de seguridad y restauración](/operations/backup-restore/).

## Reemplazar Genji

Para migrar a otra base de datos embebida:

1. Implementa `storage.Store` en un paquete nuevo.
2. Conéctalo en `cmd/mailvoid/main.go`.
3. Mantén la semántica de `CreateTable` fuera de otros paquetes; nada por encima
   de `storage` depende de los tipos de Genji.
4. Ejecuta las pruebas del repositorio compartido desde
   `internal/storage/store_test.go`.

El bus de eventos, SMTP, las políticas y las capas web no requieren cambios.
