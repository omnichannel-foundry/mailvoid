# MailVoid

**Wildcard SMTP capture server with dashboard, policies, search, WebDAV, logs.**

[English](README.md) | [Español](README.es.md)

MailVoid is a single-binary, event-first SMTP capture server. It accepts mail
for any mailbox across multiple interfaces, ports, and domains, stores envelope
metadata and MIME content in an embedded database, applies ordered regex
retention rules, keeps a traditional log per SMTP service, and streams a
structured JSON audit trail to the dashboard and API.

It is designed for development, QA, integration testing, and controlled
inspection of inbound mail. It is not an MTA, relay, IMAP/POP3 server, or
production mailbox provider.

## Features

- **Multi-interface, multi-port SMTP capture** — define listeners as
  `host:port` pairs or named services with aliases.
- **Per-service traditional logs** — each SMTP service writes its own text log
  under `<data-dir>/logs/<service>.log`.
- **Event-first audit trail** — connections, envelope commands, policy matches,
  storage, deletions, and logins become JSON events persisted to the database
  and an optional JSONL file.
- **Regex retention policies** — keep, expire after a TTL, or drop mail on
  arrival with ordered Go RE2 rules. First match wins.
- **Web console** — server-rendered dashboard with HTMX, live WebSocket events,
  message inspection, sandboxed HTML preview, and light/dark themes.
- **REST API and OpenAPI** — versioned endpoints at `/api/v1` with bearer or
  session authentication and an embedded OpenAPI 3.1 document.
- **MCP server** — Streamable HTTP at `/mcp` for agent-driven mailbox checks,
  message search, inspection, events, policies, and stats.
- **Read-only WebDAV** — browse mailboxes and download `.eml` files at `/dav/`
  with Basic auth.
- **Embedded full-text search** — Bleve index over subjects, bodies, senders,
  recipients, headers, and metadata.
- **Live capture switch** — pause and resume SMTP capture from Settings; the
  state persists across restarts.
- **Offline user administration** — list administrator accounts and reset
  passwords directly from the `mailvoid` binary.
- **Single binary** — templates, static assets, and the database engine compile
  into one executable.

## Quick start

Requirements: Go 1.24.2+ and GNU Make.

```bash
make build
./bin/mailvoid
```

MailVoid loads `./mailvoid.toml` when present and otherwise uses built-in
defaults. Open <http://localhost:8025>.

If no administrator password is configured, a random one is generated and
printed to the startup log. Configure it explicitly in `mailvoid.toml` or with
`MAILVOID_ADMIN_PASSWORD`.

Send a test message to the default SMTP listener:

```bash
python3 - <<'PY'
import smtplib

message = (
    "From: sender@example.org\r\n"
    "To: inbox@example.test\r\n"
    "Subject: Hello from MailVoid\r\n"
    "\r\n"
    "This message was captured by MailVoid.\r\n"
)

with smtplib.SMTP("127.0.0.1", 1025) as smtp:
    smtp.sendmail("sender@example.org", ["inbox@example.test"], message)
PY
```

Then open the dashboard and inspect the captured message.

## Configuration

Configuration resolves in this order, later sources winning:

1. Built-in defaults.
2. `mailvoid.toml`.
3. `MAILVOID_*` environment variables.
4. Command-line flags.

A minimal `mailvoid.toml`:

```toml
hostname = "localhost"
scheme = "http"
domains = ["*"]

[web]
bind = "0.0.0.0"
port = 8025

[smtp]
hosts = ["0.0.0.0"]
ports = [1025, 2525]

[events]
json_enabled = true
# json_path = "./data/events.jsonl"

[logging]
enabled = true
# dir = "./data/logs"

[storage]
data_dir = "./data"

[auth]
admin_user = "admin"
admin_password = ""
```

### Named SMTP services and independent logs

When each listener needs its own identity and log, use explicit services:

```toml
[[smtp.services]]
name = "dev"
host = "127.0.0.1"
port = 1025
aliases = ["local", "development"]

[[smtp.services]]
name = "qa"
host = "0.0.0.0"
port = 2525
aliases = ["staging"]
```

Services are written to `<logging.dir>/<name>.log`. The JSON event stream
remains the cross-service audit trail.

See [Configuration](docs/en/content/getting-started/configuration.md) for the full
reference, including `public_url`, reverse-proxy setups, retention limits, and
path resolution.

## API, MCP, and WebDAV

| Surface | Path | Authentication |
|---------|------|----------------|
| Dashboard | `/` | Session cookie |
| REST API | `/api/v1` | Bearer token or session cookie |
| OpenAPI | `/api/v1/openapi.json`, `/api/v1/openapi.yaml` | Public |
| MCP | `/mcp` | Bearer token |
| WebDAV | `/dav/` | Basic auth |
| Event stream | WebSocket `/ws/events`, SSE `/api/events/stream` | Session or token |

Enable the bearer token with `MAILVOID_API_TOKEN` / `[auth] api_token`, and
WebDAV with `MAILVOID_WEBDAV_PASSWORD` / `[auth] webdav_password`.

## Development

```bash
make build             # build bin/mailvoid
make test              # run the full test suite
make test-race         # run tests with the race detector
make integration       # run the JTD-driven integration suite
make lorem             # build the Lorem Ipsum SMTP load generator
make vet               # run go vet
make fmt               # format the code
make assets            # rebuild Tailwind + DaisyUI CSS (requires Node.js)
```

Templates use Tailwind utilities and DaisyUI components. The generated
`internal/web/static/app.css` is committed; run `make assets` after changing
templates or styles.

The lorem mail generator lives in `cmd/lorem-mail-generator` and sends random
messages with configurable word counts:

```bash
make lorem
./bin/lorem-mail-generator -addr 127.0.0.1:1025 -count 100 \
  -min-words 20 -max-words 200 -random-mailboxes 20 -domains example.test
```

The integration runner lives in `cmd/mailvoid-it` and can also run in a TUI:

```bash
make integration-tui
```

## Project layout

```text
cmd/mailvoid/       Main server binary
cmd/mailvoid-it/    Integration runner (batch and TUI)
internal/config/    TOML, environment, and flag configuration
internal/smtpserver/ SMTP listeners and per-service logs
internal/storage/   Storage interface, Genji, and in-memory backends
internal/web/       Fiber server, dashboard, API, MCP, WebDAV
internal/policy/    Regex retention engine
internal/events/    Event bus, JSONL sink, and event types
docs/               Hugo documentation site
testdata/           Integration test definitions
```

## Documentation

- [Introduction](docs/en/content/overview/introduction.md)
- [Features](docs/en/content/overview/features.md)
- [Configuration](docs/en/content/getting-started/configuration.md)
- [Quick start](docs/en/content/getting-started/quick-start.md)
- [API reference](docs/en/content/reference/api.md)
- [MCP](docs/en/content/reference/mcp.md)
- [Events](docs/en/content/reference/events.md)
- [Deployment](docs/en/content/operations/deployment.md)

> [!WARNING]
> MailVoid accepts every domain by default and does not implement SMTP
> authentication or STARTTLS. Never expose an unrestricted listener to the
> public internet. Restrict domains, bind to trusted interfaces, and firewall
> the SMTP ports.

## License

MIT License - Copyright (c) 2026 Antonio Ognio and contributors

Made with ❤️ from 🇵🇪. El Perú es clave 🔑.
