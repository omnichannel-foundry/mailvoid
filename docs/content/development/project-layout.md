---
title: Project layout
weight: 10
---

# Project layout

```text
.
├── cmd/
│   ├── mailvoid/
│   │   └── main.go              composition root and lifecycle
│   └── mailvoid-it/             standalone integration runner (TUI + batch)
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

## Composition root

`cmd/mailvoid/main.go` is the only place that wires implementations:

1. Load configuration.
2. Create the data directory.
3. Open the Genji store.
4. Open the JSONL event sink.
5. Build the event bus (`fileSink`, `store`).
6. Bootstrap the administrator.
7. Create the policy engine.
8. Start SMTP listeners.
9. Start the web server.
10. Start the TTL sweeper.
11. Emit `system.startup`.
12. Wait for `SIGINT`/`SIGTERM`.
13. Emit `system.shutdown` and close everything.

## Dependency direction

```text
models ← events ← storage ← policy ← smtpserver ← web ← cmd/mailvoid
```

Rules:

- `models` imports nothing from the application.
- `events` defines the bus and event type; it does not import storage.
- `storage` imports `events` only to implement `events.Sink`.
- `policy` imports `models` and `storage`.
- `smtpserver` imports `config`, `events`, `mailparse`, `models`, `policy`,
  and `storage`.
- `web` imports everything except `smtpserver`.
- No package imports `cmd/mailvoid`.

## Configuration flow

`config.Load`:

1. `Default()`.
2. Apply `MAILVOID_*` environment variables.
3. Apply flags.
4. Resolve derived paths and zero values.
5. Validate.

The web layer only reads the resolved `config.Config`. It never reads the
environment directly.

## Version metadata

`internal/build` contains:

```go
var (
    Version = "dev"
    Commit  = "none"
    Date    = "unknown"
)
```

`make build` and `make release` inject values with `-X`. `mailvoid --version`
prints them.

## Where to change what

| Change | Files |
|--------|-------|
| New config option | `internal/config/config.go`, config tests, docs |
| New event type | `internal/events/event.go`, UI event-type list, docs |
| New policy field | `internal/models/models.go`, `internal/policy/engine.go`, tests, docs |
| New message column | `internal/models/models.go`, both stores, Genji schema, API, tests |
| New legacy API route | `internal/web/server.go`, `handlers_api.go`, docs |
| New v1 API route | `internal/web/server.go`, `api_v1.go`, `openapi.yaml`, docs |
| New MCP tool | `internal/web/mcp.go`, docs + MCP test |
| WebDAV behavior | `internal/web/webdav.go` + tests |
| `.eml` download behavior | `internal/web/eml.go` + tests |
| New page | `templates/pages/`, renderer page map, handler, `server.go` |
| New shared template fragment | `templates/fragments/`, renderer auto-discovers |
| New static asset | `internal/web/static/` (embedded by `go:embed`) |
| New store method | `internal/storage/store.go`, memory, Genji, shared tests |

## Conventions

- Package-level doc comments on every package.
- Exported identifiers are documented.
- Errors are wrapped with context at the boundary that adds information.
- Persistence stays behind `storage.Store`.
- The event bus is the only way lifecycle actions become history.
- UI templates receive `fiber.Map` data with common keys (`Title`, `User`,
  `Active`, `Version`, `Flash`, `Error`).
- No comments restate obvious code; comments explain contracts and non-obvious
  decisions.

## Docs site

The Hugo Book site lives in `docs/`. The theme is vendored under
`docs/themes/hugo-book` so builds are reproducible without network access.

```bash
make docs        # build into docs/public
make docs-serve  # local preview at http://localhost:1313
```
