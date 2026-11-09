---
title: Troubleshooting
weight: 60
---

# Troubleshooting

## The service will not start

### `listen tcp ...: bind: address already in use`

Another process owns an SMTP port or the HTTP port.

```bash
ss -ltnp | grep -E '1025|2525|8025'
```

Change `MAILVOID_SMTP_PORTS` / `MAILVOID_HTTP_ADDR` or stop the other process.

### `bind: permission denied` on port 25 or 587

Ports below 1024 require privilege. Either use the unprivileged defaults, run
with `CAP_NET_BIND_SERVICE`, or forward from a privileged port with firewall
rules.

### `open database: ... resource temporarily unavailable`

Another MailVoid instance holds the Genji `LOCK` file in the data directory.
Stop the other instance or choose a different `MAILVOID_DB_PATH`.

### `configuration: ...` on startup

Read the variable name in the error. Invalid durations, booleans, byte counts,
and port ranges fail before any listener opens. See
[Configuration](/getting-started/configuration/).

## Cannot sign in

### Unknown password

If no `MAILVOID_ADMIN_PASSWORD` was configured on first run, the generated
password was printed once in the startup log. Check the journal or stdout.

If it is lost, restart once with a new password and reset enabled:

```bash
MAILVOID_ADMIN_PASSWORD='new-password' \
MAILVOID_ADMIN_RESET_PASSWORD=true \
./mailvoid
```

### Login succeeds but the next request is unauthenticated

- Behind HTTPS with `MAILVOID_COOKIE_SECURE=false`? The cookie is still sent; the
  secure flag only restricts HTTP.
- Behind HTTPS with `MAILVOID_COOKIE_SECURE=true` but browsing over HTTP? The
  browser will not send the cookie. Use HTTPS.
- A reverse proxy that strips `Set-Cookie` or rewrites the path breaks sessions.
  Ensure cookies pass through unchanged.
- The session store is in memory. Restarting MailVoid logs everyone out.

### Repeated 401 from the API

Use a cookie jar. The API does not accept bearer tokens:

```bash
curl -s -c cookies.txt -d 'username=admin&password=...' \
  http://127.0.0.1:8025/login
curl -s -b cookies.txt http://127.0.0.1:8025/api/messages
```

## SMTP clients fail

### `550 relay access denied`

The recipient domain is not in `MAILVOID_DOMAINS`. Inspect the `smtp.rcpt_to`
event payload for `accepted:false` and `reason`. Add the exact domain or a
`*.zone` pattern, or use `*` only in a trusted environment.

### `552 message too large`

The raw message exceeds `MAILVOID_MAX_MESSAGE_BYTES` (25 MiB default). Raise it
only after considering disk and memory impact.

### `451 temporary storage failure`

The database write failed. Check disk space, data-directory permissions, the
process log, and the `message.error` event. The client may retry.

### Message accepted but not stored

Check for a drop policy:

```bash
curl -s -b cookies.txt \
  'http://127.0.0.1:8025/api/events?type=message.dropped' | jq
```

The `policy.matched` event immediately before the drop shows which rule fired.

### Recipient is accepted but the body is empty

Open the raw message. If it is a malformed MIME message, MailVoid stores it with
subject `(unparsable message)`. Attachments are intentionally excluded from the
decoded bodies. HTML-only mail has an empty text body.

## Policies do not behave as expected

- **First match wins.** A broad `keep` rule with `.*` at priority 0 makes every
  later rule unreachable. Reorder or delete it.
- **Regex is case-sensitive.** Use `(?i)`.
- **RE2 has no lookaround or backreferences.** Rewrite the pattern.
- **The `any` field concatenates values.** A rule may match across field
  boundaries; test with the specific field instead.
- **TTL does not fire instantly.** The sweeper runs every
  `MAILVOID_SWEEP_INTERVAL` (default 30s) and removes messages at the next tick
  after expiry.
- **Invalid regex stored manually.** Rules saved through the API/UI are
  validated. A malformed pattern inserted directly into the database is skipped
  by the engine and never matches.

Check the actual decision:

```bash
curl -s -b cookies.txt \
  'http://127.0.0.1:8025/api/events?type=policy.matched' | jq
```

## Live events do not update

### WebSocket blocked by a proxy

The reverse proxy must pass `Upgrade` and `Connection` headers on
`/ws/events`. See [Reverse proxy](/operations/reverse-proxy/). The status dot
turns amber and the browser retries with exponential backoff.

### SSE stalls behind a proxy

Disable proxy buffering and compression for `/api/events/stream`. Nginx:
`proxy_buffering off; gzip off;`. Caddy: `flush_interval -1`.

### No events at all

`/ws/events` and `/api/events/stream` require a session cookie. The console also
needs JavaScript enabled for the live socket; the initial page still shows
persisted events.

## API, MCP, or WebDAV access fails

### `401` from `/api/v1` or `/mcp`

- The bearer token is missing or does not match `MAILVOID_API_TOKEN`.
- No token is configured, and the request has no session cookie.
- The token was rotated but the process was not restarted.
- Send `Authorization: Bearer <token>` exactly; Basic auth is not accepted here.

### `401` from `/dav/`

- `MAILVOID_WEBDAV_PASSWORD` is unset, so WebDAV is disabled and returns `404`.
- The Basic-auth username/password is wrong.
- The client sends the credentials on a redirect but not the original request;
  check the client configuration.

### `405` from `/dav/`

Only `OPTIONS`, `PROPFIND`, `GET`, and `HEAD` are allowed. The WebDAV tree is
read-only by design.

### `.eml` download returns `404`

The message ID does not exist, or the WebDAV path uses the wrong mailbox
directory. The filename is `<message-id>.eml`.

## Search returns fallback or nothing

### `backend: "fallback"`

The Bleve index is disabled or unavailable. Check:

- `MAILVOID_SEARCH_ENABLED` and `MAILVOID_SEARCH_INDEX`;
- the process startup log for `open search index` / `prepare search index`;
- event history for `search.error`.

If the index is corrupt, stop MailVoid, remove the `search.bleve` directory, and
restart; startup rebuilds it from Genji. If disk or permission errors persist,
fix those first.

### Search misses body terms

Use `GET /api/v1/search` or MCP `search_messages`; legacy
`GET /api/v1/messages?q=` is a metadata substring search. Raw MIME, base64
payloads, and attachment contents are not indexed.

## Managed credential failures

### `401` with a managed API key

The key may be deleted, expired, or mistyped. List keys with an administrator
session and `GET /api/v1/admin/api-keys`. Create a replacement and delete the
old key.

### `403` on `/api/v1/admin`

Managed API keys cannot manage credentials. Use an administrator session or the
bootstrap `MAILVOID_API_TOKEN`.

### `/dav/` returns `404` after deleting users

When no bootstrap WebDAV password and no managed users remain, the WebDAV
surface is disabled. Create a managed user through the admin API to enable it.

## Search returns nothing

- Message search covers sender, recipient, mailbox, domain, subject, and source
  IP. It does not search bodies, headers, or the raw message.
- Event search covers type, severity, and serialized payload.
- Search is case-insensitive, but `%` and `_` retain SQL `LIKE` wildcard meaning
  inside the query.

## Disk usage grows

Expected sources:

- raw messages plus decoded bodies;
- one message copy per envelope recipient;
- an append-only `events` table;
- the append-only JSONL log;
- Pebble write-ahead logs and compaction headroom.

Mitigations:

- add TTL policies;
- archive or truncate `events.jsonl` while stopped;
- export and delete old messages via the API;
- use a larger volume;
- plan an external event-retention process.

## Timestamps look wrong

Storage and JSON are UTC. The console displays UTC. The live event client
formats times in the browser timezone. If host time is wrong, fix NTP; the
database has no independent clock.

## Integration runner failures

`mailvoid-it` prints the failing step and its logs. Common causes:

- `server binary not found` — run `make build` first.
- `server did not become ready` — inspect the server log printed with the error;
  check port conflicts or a broken configuration.
- `401` — the test's admin login failed, or a created credential was already
  revoked.
- `backend` mismatch in search steps — the launched server must have
  `MAILVOID_SEARCH_ENABLED=true` (the runner sets it).
- `status = 404, want 401` for WebDAV — deleting the last managed WebDAV user
  disables `/dav/` when no bootstrap password exists; the expected status is
  `404`.

Run a single file with `-test name -batch -verbose`.

## Where to look

| Symptom | First place |
|---------|-------------|
| Process exits or fails to bind | `journalctl -u mailvoid`, stdout |
| SMTP rejection | Event stream filtered by `smtp.rcpt_to` |
| Policy decision | Event stream filtered by `policy.matched` / `message.dropped` |
| Storage failure | Event stream filtered by `message.error` / `smtp.error` |
| Login problem | Event stream filtered by `auth.login.failed` |
| Data state | `GET /api/stats` and the data directory |
