---
title: FAQ
weight: 80
---

# FAQ

## What is MailVoid?

A single-binary SMTP capture server and administrative console. It stores inbound
mail in an embedded database and exposes it through a web UI and JSON API.

## Is it a mail server?

No. It accepts SMTP transactions, stores or drops the messages, and never relays
or delivers them. There is no IMAP, POP3, or mailbox protocol.

## Does it support SMTP TLS or authentication?

No. There is no STARTTLS, implicit TLS, or SMTP AUTH. Use a private network or a
tunnel for the SMTP hop.

## Can I use port 25 or 587?

Yes, if the process has `CAP_NET_BIND_SERVICE` or sufficient privilege. The
defaults `1025` and `2525` are unprivileged.

## Can it receive mail for any domain?

Yes. `MAILVOID_DOMAINS=*` (the default) accepts every domain. Restrict this in
shared environments. Exact domains and `*.zone` wildcards are supported.

## How do I send test mail?

```bash
swaks --server 127.0.0.1:1025 \
  --from sender@example.org \
  --to inbox@example.test \
  --header "Subject: test" \
  --body "hello"
```

Or use Python's `smtplib`. See [Quick start](/getting-started/quick-start/).

## Where is data stored?

By default:

- `./data/mailvoid.db/` — Genji database directory;
- `./data/events.jsonl` — append-only JSON log.

Override with `MAILVOID_DATA_DIR`, `MAILVOID_DB_PATH`, and
`MAILVOID_EVENT_LOG`.

## How do I reset the administrator password?

```bash
MAILVOID_ADMIN_PASSWORD='new-password' \
MAILVOID_ADMIN_RESET_PASSWORD=true \
./mailvoid
```

If you omit the password with reset enabled, a new random password is generated
and printed once.

## Why did my message disappear?

Two likely causes:

1. A `drop` policy matched and the message was never stored.
2. A `ttl` policy matched and the sweeper deleted it after expiry.

Filter the event stream by `message.dropped`, `policy.matched`, or
`message.deleted`.

## Why is the live event stream not updating?

- The WebSocket or SSE route requires a valid session cookie.
- A reverse proxy may be buffering SSE or stripping WebSocket upgrade headers.
- The status dot turns amber while reconnecting.

See [Troubleshooting](/operations/troubleshooting/).

## Can I see HTML email safely?

Yes. The detail page renders HTML in a sandboxed iframe with scripts and
same-origin access disabled. Remote resources may still load inside the frame.

## Are attachments available?

They are retained inside the raw message. MailVoid does not extract attachments
into separate files or records.

## Can I search message bodies?

No. Search covers sender, recipient, mailbox, domain, subject, and source IP.
Event search covers type, severity, and serialized payload.

## How many messages can it hold?

There is no fixed limit. Capacity is disk-bound, and query cost grows with table
size because search, counts, and expiry scans are not indexed. Use TTL policies
and monitor disk.

## Can I run multiple instances on one database?

No. Genji locks the directory. Run one process per data directory. Horizontal
scaling means separate instances and separate data.

## Can I delete old events?

There is no event-deletion API. The JSONL file can be archived or truncated while
the service is stopped; the Genji `events` table retains its own history. Plan an
external retention process if needed.

## Is there a Docker image?

Not in this repository. Build the release binary and containerize it yourself if
needed; mount the data directory and set environment variables.

## Why is the binary so large?

It embeds the Go runtime, Genji/Pebble, Bleve full-text search, the full
DaisyUI/Tailwind CSS (~2.9 MiB), HTMX, Cash, and all templates. The release
build is roughly 30 MB on linux/amd64; `mailvoid-it` is a separate binary.

## Does it work on Windows or macOS?

It is Go code with no platform-specific dependencies beyond standard networking
and file paths. Build from source for your platform. These docs use Linux paths
and systemd examples.

## Why Genji?

It is an embedded, Go-native document database with no external service or CGO
requirement. The repository seam keeps it isolated, so replacing the database
does not affect the SMTP, policy, event, or web layers. See
[Storage](/reference/storage/).

## Can I use it as a spam trap?

You can capture or drop mail from untrusted sources, but there is no spam scoring,
virus scanning, or rate limiting. Restrict network access and disk usage.

## How do agents check whether mail arrived?

Use `GET /api/v1/mailboxes/{address}` with a bearer token, or the MCP tools
`check_mailbox` and `wait_for_email`. See [OpenAPI](/reference/openapi/) and
[MCP](/reference/mcp/).

## How do I create an API key or WebDAV user?

Sign in as an administrator (or use the bootstrap `MAILVOID_API_TOKEN`) and call
`POST /api/v1/admin/api-keys` or `POST /api/v1/admin/webdav-users`. The token or
generated password is returned once. See
[Managed credentials](/reference/credentials/).

## How does full-text search work?

MailVoid maintains an embedded Bleve index over subject, bodies, sender,
recipient, mailbox, domain, and headers. Use `GET /api/v1/search` or the MCP
`search_messages` tool. See [Full-text search](/reference/search/).

## Can I run this in CI?

Yes. `make integration` builds the server and the standalone `mailvoid-it`
runner, launches MailVoid on temporary ports, and executes the JTD-validated
suite. `make integration-tui` provides an interactive view. See
[Integration runner](/development/integration-runner/).

## How do I download .eml files?

Use **Download .eml** in the console, `GET /api/v1/messages/{id}/eml`, or the
read-only WebDAV tree at `/dav/`. See [WebDAV](/operations/webdav/).

## Does MailVoid support WebDAV?

Yes, read-only at `/dav/` when `MAILVOID_WEBDAV_PASSWORD` is set. It supports
`OPTIONS`, `PROPFIND`, `GET`, and `HEAD`; writes return `405`.

## How do I back up?

Stop the service and copy the whole data directory, or take an atomic filesystem
snapshot. See [Backup and restore](/operations/backup-restore/).
