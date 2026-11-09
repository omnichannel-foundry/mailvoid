---
title: Configuration
weight: 30
---

# Configuration

MailVoid resolves configuration in this order:

1. Built-in defaults.
2. The `mailvoid.toml` configuration file.
3. Environment variables.
4. Command-line flags.

Later sources win over earlier ones. Use `-config /path/to/mailvoid.toml` or
`MAILVOID_CONFIG` to load a specific file. Without either, `./mailvoid.toml` is
loaded when it exists.

## Configuration file

A complete `mailvoid.toml` looks like this:

```toml
hostname = "mail.example.com"
scheme = "https"
# public_url = "https://mail.example.com"   # overrides hostname/scheme/web.port
domains = ["*"]

[web]
bind = "127.0.0.1"
port = 8025

[smtp]
hosts = ["127.0.0.1", "10.0.0.5"]
ports = [1025, 2525]

# Optional explicit services. Each entry is one interface:port pair with its
# own identity and traditional log file. When present, services replace the
# simple hosts/ports form above.
[[smtp.services]]
name = "dev"
host = "127.0.0.1"
port = 1025
aliases = ["local", "development"]
# log_path = "/var/log/mailvoid/dev.log"

[[smtp.services]]
name = "qa"
host = "0.0.0.0"
port = 2525
aliases = ["staging"]

[events]
json_enabled = true
json_path = "./data/events.jsonl"

[logging]
enabled = true
dir = "./data/logs"

[storage]
data_dir = "./data"
# db_path = "./data/mailvoid.db"

[search]
enabled = true
# index_path = "./data/search.bleve"

[auth]
admin_user = "admin"
admin_password = ""
api_token = ""
webdav_user = "mailvoid"
webdav_password = ""
cookie_secure = false

[limits]
max_message_bytes = 26214400
read_timeout = "60s"
sweep_interval = "30s"
```

Unknown keys are rejected at startup, so typos fail fast.

## SMTP interfaces, ports, and services

There are two equivalent ways to describe SMTP listeners.

### Simple form

```toml
[smtp]
hosts = ["127.0.0.1", "10.0.0.5"]
ports = [1025, 2525]
```

This binds every interface in `hosts` on every port in `ports`. The legacy
`MAILVOID_SMTP_HOST`/`MAILVOID_SMTP_PORTS` environment variables and
`-smtp-host`/`-smtp-ports` flags use the same single-host form.

### Explicit services

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

Every service is one interface and one port. `name` is required to be unique;
`aliases` are alternative names and must also be unique. If `name` is omitted,
the first alias is used, then a generated name such as `smtp-1025` or
`smtp-127.0.0.1-2525`.

Each service writes a traditional text log to
`<logging.dir>/<service-name>.log` (or to `log_path` when configured). Events
from that listener include a `service` field, while the JSONL event log remains
the cross-service audit trail.

`MAILVOID_SMTP_HOSTS` accepts a comma-separated interface list. Setting
`MAILVOID_SMTP_HOST` or `MAILVOID_SMTP_PORTS` collapses listeners back to the
simple single-host form and regenerates service names.

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MAILVOID_CONFIG` | `./mailvoid.toml` | Configuration file path. |
| `MAILVOID_HOSTNAME` | `localhost` | Public hostname used for absolute links. |
| `MAILVOID_SCHEME` | `http` | Public URL scheme: `http` or `https`. |
| `MAILVOID_PUBLIC_URL` | *(empty)* | Full public base URL override used for generated links. |
| `MAILVOID_SMTP_HOST` | `0.0.0.0` | Single SMTP listen interface. Use `127.0.0.1` for local-only capture. |
| `MAILVOID_SMTP_HOSTS` | *(empty)* | Comma-separated SMTP listen interfaces (simple multi-interface form). |
| `MAILVOID_SMTP_PORTS` | `1025,2525` | Comma- or space-separated SMTP ports. Duplicates are removed. |
| `MAILVOID_HTTP_ADDR` | `:8025` | HTTP listen address. `:8025` binds all interfaces; `127.0.0.1:8025` binds loopback. |
| `MAILVOID_WEB_BIND` | *(from HTTP_ADDR)* | HTTP listen interface, applied after `MAILVOID_HTTP_ADDR`. |
| `MAILVOID_WEB_PORT` | *(from HTTP_ADDR)* | HTTP listen port, applied after `MAILVOID_HTTP_ADDR`. |
| `MAILVOID_DOMAINS` | `*` | Accepted recipient domains. `*` accepts all; exact names and `*.zone` patterns are supported. |
| `MAILVOID_DATA_DIR` | `./data` | Base directory for database, event log, logs, and search index. Created if missing. |
| `MAILVOID_DB_PATH` | `<data-dir>/mailvoid.db` | Genji database directory. |
| `MAILVOID_EVENT_LOG` | `<data-dir>/events.jsonl` | Append-only JSONL event log path. |
| `MAILVOID_EVENT_LOG_ENABLED` | `true` | Enable the JSONL event log. |
| `MAILVOID_LOG_ENABLED` | `true` | Enable per-service traditional text logs. |
| `MAILVOID_LOG_DIR` | `<data-dir>/logs` | Directory for per-service logs. |
| `MAILVOID_ADMIN_USER` | `admin` | Bootstrap administrator username (stored lower-case). |
| `MAILVOID_ADMIN_PASSWORD` | *(empty)* | Bootstrap password. If empty on first creation, a random password is generated and logged once. |
| `MAILVOID_ADMIN_RESET_PASSWORD` | `false` | When `true`, reset the existing administrator password to the configured/generated value. |
| `MAILVOID_SWEEP_INTERVAL` | `30s` | TTL sweeper interval. Any Go duration (`1s`, `5m`, `1h`). |
| `MAILVOID_MAX_MESSAGE_BYTES` | `26214400` | Maximum accepted message size (25 MiB). |
| `MAILVOID_READ_TIMEOUT` | `60s` | SMTP read and write timeout. |
| `MAILVOID_COOKIE_SECURE` | `false` | Mark the session cookie `Secure`. Enable when served over HTTPS. |
| `MAILVOID_API_TOKEN` | *(empty)* | Bearer token accepted by `/api/v1` and `/mcp`. Empty disables bearer auth. |
| `MAILVOID_WEBDAV_USER` | `mailvoid` | Basic-auth username for `/dav/`. |
| `MAILVOID_WEBDAV_PASSWORD` | *(empty)* | Basic-auth password for `/dav/`. Empty disables the bootstrap WebDAV credential. |
| `MAILVOID_SEARCH_ENABLED` | `true` | Enable the embedded Bleve full-text index. |
| `MAILVOID_SEARCH_INDEX` | `<data-dir>/search.bleve` | Bleve index directory. |

## Flags

Flags use the same names with a leading `-` (Go's flag package also accepts
`--`):

| Flag | Equivalent |
|------|------------|
| `-config` | `MAILVOID_CONFIG` |
| `-hostname` | `MAILVOID_HOSTNAME` |
| `-scheme` | `MAILVOID_SCHEME` |
| `-public-url` | `MAILVOID_PUBLIC_URL` |
| `-smtp-host` | `MAILVOID_SMTP_HOST` |
| `-smtp-hosts` | `MAILVOID_SMTP_HOSTS` |
| `-smtp-ports` | `MAILVOID_SMTP_PORTS` |
| `-http-addr` | `MAILVOID_HTTP_ADDR` |
| `-web-bind` | `MAILVOID_WEB_BIND` |
| `-web-port` | `MAILVOID_WEB_PORT` |
| `-domains` | `MAILVOID_DOMAINS` |
| `-data-dir` | `MAILVOID_DATA_DIR` |
| `-db-path` | `MAILVOID_DB_PATH` |
| `-event-log` | `MAILVOID_EVENT_LOG` |
| `-event-log-enabled` | `MAILVOID_EVENT_LOG_ENABLED` |
| `-log-enabled` | `MAILVOID_LOG_ENABLED` |
| `-log-dir` | `MAILVOID_LOG_DIR` |
| `-admin-user` | `MAILVOID_ADMIN_USER` |
| `-admin-password` | `MAILVOID_ADMIN_PASSWORD` |
| `-admin-reset-password` | `MAILVOID_ADMIN_RESET_PASSWORD` |
| `-sweep-interval` | `MAILVOID_SWEEP_INTERVAL` |
| `-max-message-bytes` | `MAILVOID_MAX_MESSAGE_BYTES` |
| `-read-timeout` | `MAILVOID_READ_TIMEOUT` |
| `-cookie-secure` | `MAILVOID_COOKIE_SECURE` |
| `-api-token` | `MAILVOID_API_TOKEN` |
| `-webdav-user` | `MAILVOID_WEBDAV_USER` |
| `-webdav-password` | `MAILVOID_WEBDAV_PASSWORD` |
| `-search-enabled` | `MAILVOID_SEARCH_ENABLED` |
| `-search-index` | `MAILVOID_SEARCH_INDEX` |

`--version` prints the version, commit, and build date and exits. It is handled
before configuration loading.

## Path resolution

After all sources are merged:

- If `data-dir` is empty, it becomes `./data`.
- If `db-path` is empty, it becomes `<data-dir>/mailvoid.db`.
- If `event-log` is empty, it becomes `<data-dir>/events.jsonl`.
- If `log-dir` is empty, it becomes `<data-dir>/logs`.
- If `search-index` is empty, it becomes `<data-dir>/search.bleve`.
- If `sweep-interval` is zero or negative, it becomes `30s`.
- If `max-message-bytes` is zero or negative, it becomes 25 MiB.
- If `read-timeout` is zero or negative, it becomes `60s`.

## Public links

`hostname`, `scheme`, and the web port define the absolute prefix used for
generated links. For example, `hostname = "mail.example.com"`, `scheme =
"https"`, and `[web] port = 8025` produce `https://mail.example.com:8025`.

Set `public_url` when MailVoid runs behind a reverse proxy and the public URL
differs from the listen address:

```toml
public_url = "https://mail.example.com"
```

## Domain matching

Recipient domains are lower-cased and trailing dots are removed. A domain is
accepted when any entry matches:

| Entry | Matches |
|-------|---------|
| `*` | Every domain |
| `example.test` | Exactly `example.test` |
| `*.example.test` | `example.test` and any subdomain such as `mail.example.test` |

If the domain list is empty, every domain is accepted. The default is `*`.

> [!WARNING]
> Accepting every domain (`*` or an empty list) makes MailVoid a universal SMTP
> sink. Restrict the domain list and the network exposure in any shared
> environment.

## Administrator bootstrap

On startup:

1. The configured username is lower-cased. The default is `admin`; an
   explicitly empty username fails validation before startup.
2. If the user does not exist, it is created with the configured password.
3. If the password is empty, a random 24-character URL-safe password is
   generated and printed to the log once.
4. If the user exists, the password is not changed unless
   `MAILVOID_ADMIN_RESET_PASSWORD=true`.
5. With reset enabled, the configured password is applied; if no password is
   configured, a new random password is generated and printed.

Passwords are hashed with bcrypt at cost 12. They are never stored in plaintext.

## Validation rules

Startup fails if:

- no SMTP services are configured;
- an SMTP port is outside `1–65535`;
- two SMTP services share a name or alias;
- the HTTP address is empty;
- the administrator username is empty;
- `scheme` is not `http` or `https`;
- `public_url` is not an absolute HTTP(S) URL;
- the TOML file contains unknown keys.

Invalid durations, booleans, byte counts, or port strings fail during
configuration loading with an error naming the offending value.

## Examples

### Local development only

```toml
hostname = "localhost"
scheme = "http"

[web]
bind = "127.0.0.1"
port = 8025

[smtp]
hosts = ["127.0.0.1"]
ports = [1025, 2525]
domains = ["*.test", "localhost"]

[auth]
admin_password = "dev-only-password"
```

### Several projects on one host

```toml
[web]
bind = "127.0.0.1"
port = 8025

[[smtp.services]]
name = "app"
host = "127.0.0.1"
port = 1025
aliases = ["app.local"]

[[smtp.services]]
name = "api"
host = "127.0.0.1"
port = 2525
aliases = ["api.local"]

[[smtp.services]]
name = "external"
host = "0.0.0.0"
port = 3025
aliases = ["mail.local"]

[storage]
data_dir = "/var/lib/mailvoid"

[auth]
admin_password = "use-a-secret-manager"
```

Each service logs to its own file, for example
`/var/lib/mailvoid/logs/app.log` and `/var/lib/mailvoid/logs/api.log`.

### Behind a TLS reverse proxy with agents

```toml
hostname = "mail.example.com"
scheme = "https"
public_url = "https://mail.example.com"

[web]
bind = "127.0.0.1"
port = 8025

[auth]
cookie_secure = true
api_token = "use-a-secret-manager"
webdav_user = "mailvoid"
webdav_password = "another-secret"
admin_password = "use-a-secret-manager"
```

Terminate TLS at the proxy and enable `cookie_secure`. The API token enables
`/api/v1` and `/mcp`; the WebDAV credentials enable read-only `/dav/`. See
[Reverse proxy](/operations/reverse-proxy/), [OpenAPI](/reference/openapi/),
[MCP](/reference/mcp/), and [WebDAV](/operations/webdav/).

## The data directory

| Path | Contents |
|------|----------|
| `<data-dir>/mailvoid.db/` | Genji/Pebble database directory (messages, policies, events, users, credentials) |
| `<data-dir>/events.jsonl` | Append-only JSON event log, one object per line |
| `<data-dir>/logs/<service>.log` | Traditional text log for each SMTP service |
| `<data-dir>/search.bleve/` | Rebuildable Bleve full-text index |

The database directory contains a `LOCK` file. Do not run two MailVoid processes
against the same data directory.

See [Backup and restore](/operations/backup-restore/) for safe copy procedures.
