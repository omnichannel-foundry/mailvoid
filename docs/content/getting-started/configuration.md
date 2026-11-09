---
title: Configuration
weight: 30
---

# Configuration

MailVoid resolves configuration in this order:

1. Built-in defaults.
2. Environment variables.
3. Command-line flags.

Flags win over environment variables, which win over defaults.

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MAILVOID_SMTP_HOST` | `0.0.0.0` | Address for every SMTP listener. Use `127.0.0.1` for local-only capture. |
| `MAILVOID_SMTP_PORTS` | `1025,2525` | Comma- or space-separated SMTP ports. Duplicates are removed. |
| `MAILVOID_HTTP_ADDR` | `:8025` | HTTP listen address. `:8025` binds all interfaces; `127.0.0.1:8025` binds loopback. |
| `MAILVOID_DOMAINS` | `*` | Accepted recipient domains. `*` accepts all; exact names and `*.zone` patterns are supported. |
| `MAILVOID_DATA_DIR` | `./data` | Base directory for database and event log. Created if missing. |
| `MAILVOID_DB_PATH` | `<data-dir>/mailvoid.db` | Genji database directory. |
| `MAILVOID_EVENT_LOG` | `<data-dir>/events.jsonl` | Append-only JSONL event log path. |
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
| `-smtp-host` | `MAILVOID_SMTP_HOST` |
| `-smtp-ports` | `MAILVOID_SMTP_PORTS` |
| `-http-addr` | `MAILVOID_HTTP_ADDR` |
| `-domains` | `MAILVOID_DOMAINS` |
| `-data-dir` | `MAILVOID_DATA_DIR` |
| `-db-path` | `MAILVOID_DB_PATH` |
| `-event-log` | `MAILVOID_EVENT_LOG` |
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
- If `sweep-interval` is zero or negative, it becomes `30s`.
- If `max-message-bytes` is zero or negative, it becomes 25 MiB.
- If `read-timeout` is zero or negative, it becomes `60s`.

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

- no SMTP ports are configured;
- a port is outside `1–65535`;
- the HTTP address is empty;
- the administrator username is empty.

Invalid durations, booleans, byte counts, or port strings fail during
configuration loading with an error naming the offending variable.

## Examples

### Local development only

```bash
MAILVOID_SMTP_HOST=127.0.0.1 \
MAILVOID_HTTP_ADDR=127.0.0.1:8025 \
MAILVOID_DOMAINS='*.test,localhost' \
MAILVOID_ADMIN_PASSWORD='dev-only-password' \
./mailvoid
```

### Several projects on one host

```bash
MAILVOID_SMTP_PORTS=1025,2525,3025 \
MAILVOID_DOMAINS='app.local,api.local,*.mail.local' \
MAILVOID_HTTP_ADDR=127.0.0.1:8025 \
MAILVOID_DATA_DIR=/var/lib/mailvoid \
MAILVOID_ADMIN_PASSWORD='use-a-secret-manager' \
./mailvoid
```

### Behind a TLS reverse proxy with agents

```bash
MAILVOID_HTTP_ADDR=127.0.0.1:8025 \
MAILVOID_COOKIE_SECURE=true \
MAILVOID_API_TOKEN='use-a-secret-manager' \
MAILVOID_WEBDAV_USER=mailvoid \
MAILVOID_WEBDAV_PASSWORD='another-secret' \
MAILVOID_ADMIN_PASSWORD='use-a-secret-manager' \
./mailvoid
```

Terminate TLS at the proxy and enable `MAILVOID_COOKIE_SECURE`. The API token
enables `/api/v1` and `/mcp`; the WebDAV credentials enable read-only `/dav/`.
See [Reverse proxy](/operations/reverse-proxy/), [OpenAPI](/reference/openapi/),
[MCP](/reference/mcp/), and [WebDAV](/operations/webdav/).

## The data directory

| Path | Contents |
|------|----------|
| `<data-dir>/mailvoid.db/` | Genji/Pebble database directory (messages, policies, events, users, credentials) |
| `<data-dir>/events.jsonl` | Append-only JSON event log, one object per line |
| `<data-dir>/search.bleve/` | Rebuildable Bleve full-text index |

The database directory contains a `LOCK` file. Do not run two MailVoid processes
against the same data directory.

See [Backup and restore](/operations/backup-restore/) for safe copy procedures.
