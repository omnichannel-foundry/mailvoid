---
title: Estructura del proyecto
weight: 10
---

# Estructura del proyecto

```text
.
├── cmd/
│   ├── mailvoid/
│   │   └── main.go              composition root and lifecycle
│   ├── mailvoid-it/             standalone integration runner (TUI + batch)
│   └── lorem-mail-generator/          generador de carga SMTP Lorem Ipsum aleatorio
├── internal/
│   ├── auth/                    bcrypt, verification, bootstrap
│   ├── build/                   link-time version metadata
│   ├── config/                  defaults, env, flags, validation
│   ├── events/                  event model, bus, JSONL sink
│   ├── mailbox/                 mailbox aggregation and delivery waits
│   ├── mailparse/               MIME and header parsing
│   ├── models/                  shared domain types
│   ├── policy/                  regex engine, validation, decisions
│   ├── search/                  Bleve index and IndexedStore decorator
│   ├── smtpserver/              multi-port SMTP backend and sessions
│   ├── storage/                 Store interface, memory + Genji
│   └── web/
│       ├── server.go            Fiber app, routes, static handler
│       ├── render.go            embedded template engine
│       ├── handlers_*.go        pages, legacy API, auth
│       ├── api_v1.go            versioned REST API
│       ├── mcp.go               MCP tools and Streamable HTTP handler
│       ├── webdav.go            read-only WebDAV filesystem
│       ├── eml.go               .eml downloads
│       ├── credentials.go       managed API keys and WebDAV users
│       ├── search.go            /api/v1/search and fallback
│       ├── openapi.yaml         embedded OpenAPI 3.1 document
│       ├── helpers.go           filters, pagination, form parsing
│       ├── templates/           layouts, pages, partials, fragments
│       └── static/              DaisyUI, HTMX, Cash, app CSS/JS
├── testdata/integration/        JTD schema and integration test JSON files
├── docs/                        this Hugo Book site
├── Makefile
├── go.mod
├── go.sum
└── .gitignore
```

## Raíz de composición

`cmd/mailvoid/main.go` es el único lugar que conecta implementaciones:

1. Carga la configuración.
2. Crea el directorio de datos.
3. Abre el almacén Genji.
4. Abre el destino de eventos JSONL.
5. Construye el bus de eventos (`fileSink`, `store`).
6. Inicializa el administrador.
7. Crea el motor de políticas.
8. Inicia los listeners SMTP.
9. Inicia el servidor web.
10. Inicia el barrido TTL.
11. Emite `system.startup`.
12. Espera a `SIGINT`/`SIGTERM`.
13. Emite `system.shutdown` y cierra todo.

## Dirección de dependencias

```text
models ← events ← storage ← policy ← smtpserver ← web ← cmd/mailvoid
```

Reglas:

- `models` no importa nada de la aplicación.
- `events` define el bus y el tipo de evento; no importa storage.
- `storage` importa `events` solo para implementar `events.Sink`.
- `policy` importa `models` y `storage`.
- `smtpserver` importa `config`, `events`, `mailparse`, `models`, `policy` y
  `storage`.
- `web` importa todo excepto `smtpserver`.
- Ningún paquete importa `cmd/mailvoid`.

## Flujo de configuración

`config.Load`:

1. `Default()`.
2. Aplica las variables de entorno `MAILVOID_*`.
3. Aplica las flags.
4. Resuelve rutas derivadas y valores cero.
5. Valida.

La capa web solo lee el `config.Config` resuelto. Nunca lee el entorno
directamente.

## Metadatos de versión

`internal/build` contiene:

```go
var (
    Version = "dev"
    Commit  = "none"
    Date    = "unknown"
)
```

`make build` y `make release` inyectan valores con `-X`. `mailvoid --version` los
imprime.

## Dónde cambiar cada cosa

| Cambio | Archivos |
|--------|----------|
| Nueva opción de configuración | `internal/config/config.go`, pruebas de configuración, documentación |
| Nuevo tipo de evento | `internal/events/event.go`, lista de tipos de evento de la UI, documentación |
| Nuevo campo de política | `internal/models/models.go`, `internal/policy/engine.go`, pruebas, documentación |
| Nueva columna de mensaje | `internal/models/models.go`, ambos almacenes, esquema Genji, API, pruebas |
| Nueva ruta de API heredada | `internal/web/server.go`, `handlers_api.go`, documentación |
| Nueva ruta de API v1 | `internal/web/server.go`, `api_v1.go`, `openapi.yaml`, documentación |
| Nueva herramienta MCP | `internal/web/mcp.go`, documentación + prueba MCP |
| Comportamiento de WebDAV | `internal/web/webdav.go` + pruebas |
| Comportamiento de descarga `.eml` | `internal/web/eml.go` + pruebas |
| Nueva página | `templates/pages/`, mapa de páginas del renderizador, handler, `server.go` |
| Nuevo fragmento de plantilla compartido | `templates/fragments/`, el renderizador lo descubre automáticamente |
| Nuevo recurso estático | `internal/web/static/` (incrustado con `go:embed`) |
| Nuevo método del almacén | `internal/storage/store.go`, memoria, Genji, pruebas compartidas |

## Convenciones

- Comentarios de documentación a nivel de paquete en cada paquete.
- Los identificadores exportados están documentados.
- Los errores se envuelven con contexto en el límite que añade información.
- La persistencia permanece detrás de `storage.Store`.
- El bus de eventos es la única forma en que las acciones del ciclo de vida se
  convierten en historial.
- Las plantillas de la UI reciben datos `fiber.Map` con claves comunes (`Title`,
  `User`, `Active`, `Version`, `Flash`, `Error`).
- Ningún comentario repite código obvio; los comentarios explican contratos y
  decisiones no evidentes.

## Sitio de documentación

El sitio Hugo Book vive en `docs/`. El tema está incluido en el repositorio bajo
`docs/themes/hugo-book` para que las compilaciones sean reproducibles sin acceso a
la red.

```bash
make docs        # build into docs/public
make docs-serve  # local preview at http://localhost:1313
```
