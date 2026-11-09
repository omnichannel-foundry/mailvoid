---
title: Features
weight: 20
---

# Features

## SMTP capture

- Multiple simultaneous SMTP listeners on configurable ports.
- Wildcard mailbox acceptance with exact-domain, zone-wildcard, and accept-all
  domain policies.
- Per-message envelope capture: source IP, receiving port, recipient domain and
  mailbox, envelope sender, timestamp, and size.
- Raw message retention plus decoded subject, plain-text body, and HTML body.
- MIME handling for multipart messages, base64, quoted-printable, RFC 2047
  encoded subjects, and common charsets.
- One stored message per envelope recipient, so a policy can treat
  `alice@example.test` differently from `bob@example.test`.
- Graceful handling of unparsable messages: the raw bytes are still stored and a
  warning event is emitted.

## Retention policies

- Ordered regex rules with first-match-wins evaluation.
- Target fields: `sender`, `recipient`, `mailbox`, `domain`, `subject`,
  `remote_ip`, `local_port`, and `any`.
- Actions: `keep` permanently, `ttl` auto-delete after a duration, and `drop`
  blackhole on arrival.
- Go RE2 syntax: linear-time matching, no backreferences or lookaround.
- Create, edit, enable/disable, reorder, delete, and test rules from the UI or
  the JSON API.
- TTL sweeper deletes expired messages and emits `message.deleted` with reason
  `ttl expiry`.

## API, MCP, and file access

- Versioned REST API at `/api/v1` with an embedded OpenAPI 3.1.0 document.
- Bearer-token authentication for agents and automation.
- MCP server at `/mcp` over stateless Streamable HTTP, with tools for mailbox
  checks, delivery waits, message inspection, search, events, policies, stats,
  and deletion.
- Raw `.eml` downloads from the message detail page and
  `GET /api/v1/messages/{id}/eml`.
- Read-only WebDAV at `/dav/` with Basic auth: list mailboxes and download
  `<message-id>.eml` files from any WebDAV client.

## Full-text search

- Embedded Bleve index over subject, decoded bodies, sender, recipient,
  mailbox, domain, source IP, and headers.
- BM25 ranking, highlights, metadata filters, and pagination through
  `GET /api/v1/search` and the MCP `search_messages` tool.
- Rebuildable projection of Genji; failures are reported as `search.error` and
  do not block mail capture.
- Substring metadata search remains as a fallback when the index is unavailable.

## Offline user administration

- List administrator accounts with `mailvoid users list`.
- Reset an administrator password with `mailvoid users passwd [username]`.
- Commands open the database directly, so stop the server first; the database
  uses an exclusive lock.

## Managed credentials

- Administrator API endpoints to create, list, and revoke API keys and WebDAV
  users.
- API keys are shown once and stored as SHA-256 hashes.
- WebDAV passwords are bcrypt-hashed and generated when omitted.
- Managed credentials take effect immediately; no restart required.

## Event system

- Structured events with ID, UTC timestamp, type, severity, and JSON payload.
- Persisted to both an append-only JSONL file and a Genji `events` table.
- Broadcast to browsers over WebSocket with an SSE alternative for scripts.
- Bounded in-memory ring for very recent events.
- Event coverage for system lifecycle, SMTP lifecycle, message lifecycle,
  policies, and authentication.

## Web console

- Server-rendered pages with HTMX partial updates.
- DaisyUI/Tailwind styling with a persistent light/dark theme toggle.
- Dashboard with counts, recent messages, and recent events.
- Message search and filters by domain, mailbox, port, and action.
- Message detail with sandboxed HTML preview, text body, headers, raw message,
  matched policy, and a live expiry countdown.
- Policy management panel with create, edit, toggle, reorder, delete, and a
  regex tester.
- Live event stream with pause and clear controls.
- Cash-powered clipboard copy, theme switching, and mobile drawer behavior.

## Authentication and administration

- bcrypt password hashes (cost 12).
- Cookie-based sessions with HTTP-only, SameSite=Lax defaults.
- One bootstrap administrator, configurable through environment variables or
  flags; a random password is generated and printed once if none is supplied.
- Password change from the settings page.
- Every authenticated account has full administrator access.
- All UI routes and API endpoints are protected, except `/healthz`, `/login`,
  and embedded static assets.

## Integration runner

- Standalone `mailvoid-it` binary that launches MailVoid and exercises REST,
  MCP, SMTP, WebDAV, `.eml`, credentials, and search.
- Interactive Charmbracelet TUI and CI-friendly batch mode.
- Tests are JTD-validated JSON files under `testdata/integration`.

## Operations

- Single static binary; no Node, no CGO requirement for release builds.
- Embedded database and assets.
- Graceful shutdown of HTTP, SSE, WebSocket, and SMTP listeners.
- Environment-variable and flag configuration.
- JSON API for messages, events, policies, stats, and regex tests.
- Persistence across restarts.
