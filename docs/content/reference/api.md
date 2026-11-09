---
title: HTTP API
weight: 20
---

# HTTP API

> [!NOTE]
> The canonical machine API is **`/api/v1`**, documented in
> [OpenAPI](/reference/openapi/) and consumed by [MCP](/reference/mcp/). It adds
> bearer-token auth, mailbox summaries, delivery waits, and `.eml` download. The
> `/api` paths on this page remain available for the console and existing
> session-authenticated clients.

All endpoints are served by the same Fiber process as the console. JSON
responses use UTF-8 and RFC 3339 timestamps.

## Authentication

Every endpoint except `GET /healthz`, `GET /static/*`, and the login routes
requires a valid session cookie. Obtain one by posting the login form:

```bash
curl -s -c cookies.txt \
  -d 'username=admin&password=change-me-please' \
  http://localhost:8025/login

curl -s -b cookies.txt http://localhost:8025/api/messages
```

Unauthenticated API requests return:

```http
HTTP/1.1 401 Unauthorized
Content-Type: application/json

{"error":"authentication required"}
```

These legacy `/api` routes require the session cookie. Bearer tokens are accepted only by `/api/v1` and `/mcp`.

### Session endpoints

| Method | Path | Body | Result |
|--------|------|------|--------|
| `GET` | `/login` | — | Login page (redirects home if already signed in) |
| `POST` | `/login` | form: `username`, `password`, optional `next` | `302` + `Set-Cookie` on success; `401` login page on failure |
| `POST` | `/logout` | — | Destroys the session, `302` to `/login` |

`next` is accepted only when it is a local path beginning with `/` and not `//`.
External redirects are rejected.

## Errors

All API errors use:

```json
{"error":"human-readable message"}
```

| Status | Meaning |
|--------|---------|
| `400` | Invalid JSON, invalid regex, unknown field/action, bad reorder list |
| `401` | Missing or expired session |
| `404` | Message or policy not found |
| `500` | Storage or internal failure |

## Pagination and filters

List endpoints accept:

| Parameter | Default | Limits |
|-----------|---------|--------|
| `page` | `1` | 1-based |
| `limit` | `50` | 1–200 |

Responses include `count` (total matches), `limit`, and `offset`.

## Messages

### `GET /api/messages`

List message metadata, newest first. Bodies and raw content are omitted from the
list; fetch the detail endpoint for those.

Query parameters:

| Parameter | Description |
|-----------|-------------|
| `q` | Case-insensitive substring over sender, recipient, mailbox, domain, subject, and remote IP |
| `domain` | Exact domain |
| `mailbox` | Exact local part |
| `action` | `keep` or `ttl` |
| `port` | Exact receiving port |
| `page`, `limit` | Pagination |

```bash
curl -s -b cookies.txt \
  'http://localhost:8025/api/messages?q=invoice&port=1025&limit=20'
```

```json
{
  "messages": [
    {
      "id": "msg_81997d51ccdda1a5",
      "received_at": "2026-11-08T01:28:24.083Z",
      "remote_ip": "203.0.113.9",
      "local_port": 1025,
      "domain": "example.test",
      "mailbox": "inbox",
      "recipient": "inbox@example.test",
      "sender": "sender@example.org",
      "subject": "Invoice 42",
      "size": 552,
      "action": "ttl",
      "policy_id": "pol_a89e827c828431fe",
      "policy_name": "expire invoices",
      "expires_at": "2026-11-09T01:28:24Z"
    }
  ],
  "count": 1,
  "limit": 20,
  "offset": 0
}
```

`policy_id`, `policy_name`, and `expires_at` are omitted when empty.

### `GET /api/messages/:id`

Return the full message record.

```json
{
  "id": "msg_81997d51ccdda1a5",
  "received_at": "2026-11-08T01:28:24.083Z",
  "remote_ip": "203.0.113.9",
  "local_port": 1025,
  "domain": "example.test",
  "mailbox": "inbox",
  "recipient": "inbox@example.test",
  "sender": "sender@example.org",
  "subject": "Invoice 42",
  "headers": {
    "Content-Type": ["multipart/alternative; boundary=..."],
    "Subject": ["Invoice 42"]
  },
  "raw_headers": "Content-Type: ...\r\nSubject: Invoice 42\r\n",
  "raw": "Content-Type: ...\r\n\r\n--boundary...",
  "body_text": "plain text body",
  "body_html": "<p>HTML body</p>",
  "size": 552,
  "policy_id": "pol_a89e827c828431fe",
  "policy_name": "expire invoices",
  "action": "ttl",
  "expires_at": "2026-11-09T01:28:24Z",
  "deleted": false
}
```

### `DELETE /api/messages/:id`

Permanently delete a message and emit `message.deleted` with reason
`api deletion`.

```json
{"deleted": true, "id": "msg_81997d51ccdda1a5"}
```

## Events

### `GET /api/events`

List events newest first.

| Parameter | Description |
|-----------|-------------|
| `type` | Exact event type, e.g. `message.dropped` |
| `severity` | `debug`, `info`, `warn`, or `error` |
| `q` | Case-insensitive substring over type, severity, and payload JSON |
| `page`, `limit` | Pagination |

```json
{
  "events": [
    {
      "id": "ev_9f3c1d2a7b4e6f80",
      "timestamp": "2026-11-08T01:28:24.083Z",
      "type": "message.received",
      "severity": "info",
      "payload": {
        "message_id": "msg_81997d51ccdda1a5",
        "remote_ip": "203.0.113.9",
        "local_port": 1025,
        "domain": "example.test",
        "mailbox": "inbox",
        "recipient": "inbox@example.test",
        "sender": "sender@example.org",
        "subject": "Invoice 42",
        "size": 552,
        "action": "ttl"
      }
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### `GET /api/events/stream`

Server-Sent Events. The first frame is `event: ready`; subsequent frames use the
event type as the SSE event name. A comment ping is sent every 20 seconds.

```text
event: ready
data: {"ok":true}

event: smtp.connected
data: {"id":"ev_...","timestamp":"...","type":"smtp.connected","severity":"info","payload":{...}}
```

The stream has no replay support. Use `GET /api/events` for history.

### `GET /ws/events`

WebSocket upgrade. Requires the session cookie on the handshake. Each event is a
JSON text frame. The server pings every 25 seconds.

```javascript
const ws = new WebSocket("ws://localhost:8025/ws/events");
ws.onmessage = (message) => {
  const event = JSON.parse(message.data);
  console.log(event.type, event.payload);
};
```

Browser clients send cookies automatically. Non-browser clients must set the
`Cookie` header.

## Policies

### `GET /api/policies`

```json
{
  "policies": [
    {
      "id": "pol_366af449b712cc77",
      "name": "drop spam",
      "field": "sender",
      "pattern": "@spam\\.test$",
      "action": "drop",
      "ttl_seconds": 0,
      "priority": 0,
      "enabled": true,
      "created_at": "2026-11-08T01:28:30.373Z",
      "updated_at": "2026-11-08T01:28:30.373Z"
    }
  ],
  "count": 1
}
```

### `POST /api/policies`

Create a policy. `enabled` defaults to `true` when omitted. `priority` is
appended after the current last rule when omitted or zero.

```bash
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{
        "name": "drop spam",
        "field": "sender",
        "pattern": "@spam\\.test$",
        "action": "drop",
        "enabled": true
      }' \
  http://localhost:8025/api/policies
```

Returns `201 Created` with the stored policy.

Validation:

- `name` required;
- `field` must be a known field;
- `action` must be `keep`, `ttl`, or `drop`;
- `pattern` must compile and be at most 2048 characters;
- `ttl_seconds > 0` is required for `ttl` and is normalized to `0` otherwise.

### `GET /api/policies/:id`

Return one policy or `404`.

### `PUT /api/policies/:id`

Full replacement update for the rule's text fields. Missing strings and
`ttl_seconds` become zero values, which commonly fails validation. Missing
`enabled` and `priority` preserve the existing values. `id` and `created_at` are
always preserved.

### `DELETE /api/policies/:id`

```json
{"deleted": true, "id": "pol_366af449b712cc77"}
```

### `POST /api/policies/reorder`

The `ids` array must contain every policy exactly once. Priorities become the
array index (`0` first).

```bash
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"ids":["pol_b","pol_a","pol_c"]}' \
  http://localhost:8025/api/policies/reorder
```

```json
{"reordered": true}
```

### `POST /api/policies/test`

Evaluate a pattern against a sample value without saving a rule.

```bash
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"field":"subject","pattern":"^invoice","value":"Invoice 42"}' \
  http://localhost:8025/api/policies/test
```

```json
{"field":"subject","matched":true,"pattern":"^invoice","value":"Invoice 42"}
```

Unknown fields and invalid regexes return `400`.

## Statistics

### `GET /api/stats`

```json
{
  "messages": 42,
  "messages_24h": 7,
  "policies": 3,
  "events": 512,
  "version": "v1.2.3",
  "uptime": "1h2m3s"
}
```

## Health

### `GET /healthz`

Public, no authentication.

```json
{"status":"ok","version":"v1.2.3","uptime":"1h2m3s"}
```

## Static assets

`GET /static/*` serves the embedded DaisyUI CSS, HTMX, Cash, local CSS, and local
JavaScript with a one-hour cache and `X-Content-Type-Options: nosniff`.
