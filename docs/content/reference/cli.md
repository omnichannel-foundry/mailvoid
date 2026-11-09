---
title: CLI and flags
weight: 10
---

# CLI and flags

MailVoid has no subcommands. It starts the SMTP listeners, HTTP server, sweeper,
and event sinks in one process.

```text
mailvoid [flags]
mailvoid --version
```

`--version` prints the injected version, commit, and build date, then exits
before configuration loading.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-smtp-host` | string | `0.0.0.0` | SMTP listen host |
| `-smtp-ports` | string | `1025,2525` | Comma/space-separated ports |
| `-http-addr` | string | `:8025` | HTTP listen address |
| `-domains` | string | `*` | Accepted domains |
| `-data-dir` | string | `./data` | Data directory |
| `-db-path` | string | `<data-dir>/mailvoid.db` | Genji database directory |
| `-event-log` | string | `<data-dir>/events.jsonl` | JSONL event log |
| `-admin-user` | string | `admin` | Bootstrap username |
| `-admin-password` | string | *(empty)* | Bootstrap password |
| `-admin-reset-password` | bool | `false` | Reset an existing admin password |
| `-sweep-interval` | duration | `30s` | TTL sweep interval |
| `-max-message-bytes` | int64 | `26214400` | Maximum message size |
| `-read-timeout` | duration | `60s` | SMTP read/write timeout |
| `-cookie-secure` | bool | `false` | Secure session cookie |
| `-api-token` | string | *(empty)* | Bearer token for `/api/v1` and `/mcp` |
| `-webdav-user` | string | `mailvoid` | WebDAV basic-auth username |
| `-webdav-password` | string | *(empty)* | WebDAV bootstrap password; empty disables the bootstrap credential |
| `-search-enabled` | bool | `true` | Enable Bleve full-text search |
| `-search-index` | string | `<data-dir>/search.bleve` | Search index directory |

Go's flag package accepts both `-flag` and `--flag`. Boolean flags accept
`-cookie-secure`, `-cookie-secure=true`, and `-cookie-secure=false`.

## Environment

Every flag has a `MAILVOID_` environment equivalent. Flags override environment
values. See [Configuration](/getting-started/configuration/) for the complete
table, path resolution, and validation rules.

## Exit behavior

| Situation | Result |
|-----------|--------|
| `--version` | Prints metadata, exit 0 |
| Invalid configuration | Logs `configuration: ...`, exit 1 |
| Database/event-log open failure | Logs the error, exit 1 |
| SMTP listener bind failure | Logs the error, exit 1 |
| HTTP listener failure | Logs the error, shuts down cleanly, exit 1 |
| `SIGINT` / `SIGTERM` | Graceful shutdown, exit 0 |

## Signals

`SIGINT` and `SIGTERM` trigger:

1. Stop accepting new work (context cancellation).
2. Stop the TTL sweeper.
3. Emit `system.shutdown`.
4. Close HTTP, SSE, and WebSocket connections.
5. Close SMTP listeners and active connections.
6. Flush and close the JSONL event log.
7. Close the Genji database.

## Make targets

| Target | Description |
|--------|-------------|
| `make build` | Build `bin/mailvoid` with version metadata |
| `make release` | Static, stripped release binary |
| `make run` | Build and run (`ARGS='...'` for flags) |
| `make test` | Run all tests |
| `make test-race` | Race detector, uncached |
| `make cover` | HTML coverage report |
| `make vet` | `go vet` |
| `make fmt` | gofumpt if installed, otherwise gofmt |
| `make fmt-check` | Fail if formatting differs |
| `make lint` | golangci-lint (must be installed) |
| `make check` | fmt-check + vet + test |
| `make tidy`, `make tidy-check` | Module tidiness |
| `make it` | Build the standalone integration runner |
| `make integration` | Run the JTD suite in batch mode |
| `make integration-tui` | Run the JTD suite in the interactive TUI |
| `make install`, `make uninstall` | Install/remove under `PREFIX/bin` |
| `make clean` | Remove `bin/` and coverage output |
