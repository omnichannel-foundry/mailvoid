---
title: MCP server
weight: 25
---

# MCP server

MailVoid exposes a read-oriented Model Context Protocol server at `/mcp` so
agents can verify delivery, inspect messages, and read the audit trail without
constructing REST requests.

## Transport

- **Streamable HTTP** at `/mcp` (`POST` JSON-RPC; the server returns JSON
  responses rather than SSE streams).
- **Stateless**: no `Mcp-Session-Id` continuity is required.
- The MCP server identifies itself as `mailvoid` with the MailVoid build
  version.

## Authentication

`/mcp` uses the same agent authentication as `/api/v1`:

- `Authorization: Bearer <MAILVOID_API_TOKEN>`; or
- the `mailvoid_session` cookie.

Set a token before exposing the endpoint:

```bash
MAILVOID_API_TOKEN='replace-with-a-long-random-token' ./mailvoid
```

If no token is configured, only a browser/session-authenticated client can use
MCP. See [Configuration](/getting-started/configuration/).

## Client configuration

The exact shape depends on the MCP client. For clients that accept a remote
streamable HTTP server, the essential fields are the URL and the bearer header:

```json
{
  "mcpServers": {
    "mailvoid": {
      "url": "http://localhost:8025/mcp",
      "headers": {
        "Authorization": "Bearer replace-with-your-token"
      }
    }
  }
}
```

Some clients call the transport `type: "http"` or require a separate `headers`
object. Consult your client's documentation; the URL and header are the parts
MailVoid requires.

## Tools

| Tool | Purpose |
|------|---------|
| `list_mailboxes` | List mailboxes with counts and latest-message metadata |
| `check_mailbox` | Check delivery for one mailbox and return recent messages |
| `wait_for_email` | Wait for a new message, with a timeout |
| `get_message` | Read one message, decoded bodies, headers, and optional raw content |
| `search_messages` | Query messages with the REST filters |
| `list_events` | Read audit events by type, severity, or search text |
| `list_policies` | Read retention rules in order |
| `get_stats` | Read capture counts, version, and uptime |
| `delete_message` | Permanently delete a message and emit an audit event |

### `check_mailbox`

Input:

```json
{"address": "inbox@example.test", "since": "2026-11-01T00:00:00Z", "limit": 10}
```

Output:

```json
{
  "found": true,
  "mailbox": {
    "address": "inbox@example.test",
    "mailbox": "inbox",
    "domain": "example.test",
    "message_count": 2,
    "latest_subject": "Welcome",
    "latest_received_at": "2026-11-08T01:28:24.083Z"
  },
  "messages": []
}
```

`found` is `true` when the mailbox has at least one captured message after
`since` (or of all time when `since` is omitted).

### `wait_for_email`

Input:

```json
{
  "address": "inbox@example.test",
  "since": "2026-11-08T01:00:00Z",
  "timeout_seconds": 30
}
```

The timeout is clamped to 1–60 seconds. On timeout the tool succeeds with
`{"found": false, "waited_seconds": ...}`. On delivery it returns a message
summary.

### `get_message`

```json
{"id": "msg_81997d51ccdda1a5", "include_raw": true}
```

`include_raw` adds the complete RFC 5322 message to the result. Without it, the
tool returns metadata, headers, raw headers, and decoded text/HTML bodies.

### `search_messages`

Accepts `q`, `domain`, `mailbox`, `recipient`, `sender`, `action`, `port`,
`since`, `until`, `limit`, `offset`, and `sort` (`relevance` or
`received_at`). With the search index enabled, `q` is a Bleve full-text query
over subject, decoded body, sender, recipient, mailbox, domain, and headers.
Results include `messages`, `count`, `backend` (`bleve` or `fallback`), and
optional `highlights`. See [Full-text search](/reference/search/).

### `delete_message`

Permanently removes a message and emits `message.deleted` with
`reason: "mcp deletion"` and `username: "mcp"`. Treat this tool as destructive.

## Example JSON-RPC initialization

MCP clients perform initialization automatically. The raw request shape is:

```bash
curl -s http://localhost:8025/mcp \
  -H 'Authorization: Bearer replace-with-your-token' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
          "protocolVersion": "2025-06-18",
          "capabilities": {},
          "clientInfo": {"name": "curl", "version": "1"}
        }
      }'
```

Then use `tools/list` and `tools/call` as defined by the MCP specification. An
MCP SDK client is recommended over hand-written JSON-RPC.

## Security

- The MCP endpoint can read every captured message and delete messages. Protect
  the token like an administrator password.
- Prefer HTTPS through a reverse proxy; bearer tokens over plain HTTP can be
  observed on the network.
- The server is stateless, so there is no MCP session to revoke. Rotate the API
  token and restart MailVoid to invalidate old tokens.
- Tool calls are not separately audit-logged. The operations they perform (for
  example, deletion) emit their normal events.

## What MCP does not expose

- Policy creation or editing. That remains available through the console and the
  REST API.
- Event streaming. Use `/ws/events`, `/api/events/stream`, or the console for
  live data.
- Raw `.eml` file download as a separate tool. Use `get_message` with
  `include_raw: true`, or the REST and WebDAV download paths.
