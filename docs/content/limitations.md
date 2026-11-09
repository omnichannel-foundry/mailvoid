---
title: Limitations
weight: 70
---

# Limitations

MailVoid is intentionally narrow. These are known constraints, not bugs.

## SMTP

- **No STARTTLS or implicit TLS.** SMTP traffic is plaintext.
- **No SMTP AUTH.** Anyone who can reach a listener can submit mail.
- **No relay or outbound delivery.** MailVoid never forwards mail.
- **No SMTPUTF8, DSN, or other optional extensions advertised.** The base
  go-smtp transaction is used.
- **No per-client rate limiting or connection limits.**
- **No TLS handshake event** exists because TLS is not implemented. The event
  model covers every lifecycle action the server actually performs.
- **One stored copy per envelope recipient.** Large mail to many recipients
  multiplies storage.

## MIME

- Attachments are retained only inside the raw message; they are not extracted
  into separate records.
- `message/rfc822` parts are not recursively parsed.
- Charset is taken from the MIME parameter; HTML `<meta charset>` is ignored.
- Only text/plain and text/html bodies are decoded.
- No HTML sanitization is performed on the stored HTML. The console isolates it
  in a sandboxed iframe instead.

## Search

- Search is a substring match over metadata (and event payloads), not full-text
  search over bodies.
- The Genji repository uses `LIKE` without secondary indexes; large tables make
  search and counts slower.
- `%` and `_` retain SQL wildcard meaning in queries.

## Events

- Event and JSONL history are append-only and have no built-in retention or
  compaction.
- Live transports have no replay or `Last-Event-ID` support.
- Event sink failures are ignored so an audit failure cannot block capture.
- Slow live subscribers drop events instead of applying backpressure.
- There is no event deletion API.

## Search

- Full-text search uses Bleve v2.5.7, the newest release compatible with Go
  1.24; newer Bleve releases require Go 1.25.
- Raw MIME, base64 payloads, and attachment contents are not indexed. HTML is
  stripped to text.
- The index is a synchronous projection. Index failures do not block capture,
  but search can be stale until a rebuild.
- When the index is disabled or unavailable, `/search` and MCP fall back to the
  legacy metadata substring search, which does not search bodies.

## API and agent surfaces

- `/api/v1` and `/mcp` share one global bearer token. There are no per-agent
  scopes, tokens, or audit identities beyond `api-token`.
- MCP is stateless and returns JSON responses; it does not expose event
  streaming or policy mutation tools.
- WebDAV is read-only and uses one global Basic-auth credential. It has no
  per-mailbox authorization, locking, or property writes.
- `.eml` downloads return the stored raw bytes; attachments are not extracted
  separately.

## Authentication and administration

- Every authenticated account is an administrator; there are no roles or
  per-mailbox permissions.
- There is no user-management UI or API for creating additional accounts.
- Sessions are stored in memory and reset on restart.
- No CSRF tokens are implemented; the session cookie is `SameSite=Lax`.
- No login rate limiting, lockout, or audit of password strength.
- No API tokens; session cookies only.

## Storage

- Genji v0.15.3 is pinned to the last release under the `genjidb/genji` module
  path; upstream development moved to Chai.
- The repository stays within a small Genji SQL subset: one `ORDER BY`
  expression, no `LOWER()`, no native upsert.
- No schema migration framework. Schema changes require a deliberate migration.
- One process per data directory; Genji holds a lock.
- Database and event log are unencrypted at rest.
- No built-in backup scheduler.

## Operations

- No official container image or packaging beyond `make install`.
- No clustering or horizontal scaling of one dataset.
- No built-in metrics endpoint (Prometheus or otherwise); use `/healthz`,
  `/api/stats`, logs, and host metrics.
- No built-in TLS for the dashboard; use a reverse proxy.
- No configurable Pebble cache or storage tuning.
- No SMTP proxy support inside the process.

## Frontend

- The binary embeds the full DaisyUI/Tailwind bundle (~2.9 MiB), which dominates
  first-load size.
- There is no Node build pipeline; changing assets means replacing pinned files
  and rebuilding the binary.
- The live event page uses WebSocket; JavaScript-disabled browsers see the
  initial persisted rows but no live updates.
- The dashboard is not localized.

## Product scope

- Not an MTA, IMAP/POP3 server, mailbox provider, or spam filter.
- Drop policies are silent blackholes; senders receive success.
- No message deduplication or idempotency keys.
- No delivery guarantees beyond the SMTP transaction response.
