---
title: Event stream
weight: 40
---

# Event stream

MailVoid records every significant lifecycle action as a structured event.
Events are persisted to two places and broadcast to live consumers.

## Event shape

```json
{
  "id": "ev_9f3c1d2a7b4e6f80",
  "timestamp": "2026-11-08T01:28:24.083Z",
  "type": "message.received",
  "severity": "info",
  "payload": {
    "message_id": "msg_81997d51ccdda1a5",
    "remote_ip": "127.0.0.1",
    "local_port": 12525,
    "domain": "example.test",
    "mailbox": "inbox",
    "recipient": "inbox@example.test",
    "sender": "sender@example.org",
    "subject": "Hello from port 12525",
    "size": 552,
    "action": "keep"
  }
}
```

| Field | Description |
|-------|-------------|
| `id` | `ev_` plus 16 random hex characters |
| `timestamp` | UTC time, RFC 3339 in JSON |
| `type` | Dotted event type such as `smtp.rcpt_to` |
| `severity` | `debug`, `info`, `warn`, or `error` |
| `payload` | Event-specific JSON object; omitted when empty |

## Persistence

- **JSONL file** — one event per line at `MAILVOID_EVENT_LOG`
  (`<data-dir>/events.jsonl` by default). Append-only.
- **Genji `events` table** — queryable history used by the console and API.
- **In-memory ring** — the last 256 events, used for very recent views.

Sinks are called synchronously before an event is broadcast. Live subscribers
receive events on a best-effort basis; slow consumers drop events instead of
blocking the server. The JSONL file and database are the durable record.

## Live transports

### WebSocket

`GET /ws/events` upgrades to a WebSocket and sends each event as a JSON text
frame. The console uses this transport.

- requires a valid session cookie;
- server sends ping frames every 25 seconds;
- write deadline is 10 seconds;
- no replay: events emitted before the connection are not resent.

### Server-Sent Events

`GET /api/events/stream` streams:

```text
event: ready
data: {"ok":true}

event: smtp.connected
data: {"id":"ev_...","timestamp":"...","type":"smtp.connected","severity":"info","payload":{...}}

: ping
```

- requires a valid session cookie;
- heartbeat comment every 20 seconds;
- no replay / `Last-Event-ID` support.

Use the history endpoint if you need events that occurred before connecting.

## Console

The **Event stream** page:

- loads the most recent persisted events (default 50);
- prepends new events over WebSocket;
- shows a green live dot while connected and an amber dot while reconnecting;
- supports **Pause** (buffers up to 200 events in the browser) and **Resume**;
- supports **Clear view** (does not delete server-side events);
- keeps at most 500 rows in the browser DOM.

Filters for type, severity, and payload search are applied server-side on page
load. New live events are not filtered client-side; clear the view or reload
after changing filters.

## History and filters

```bash
# newest first
curl -s -b cookies.txt 'http://localhost:8025/api/events?limit=50'

# by type
curl -s -b cookies.txt \
  'http://localhost:8025/api/events?type=message.dropped'

# by severity
curl -s -b cookies.txt \
  'http://localhost:8025/api/events?severity=error'

# substring search over type, severity, and serialized payload
curl -s -b cookies.txt \
  'http://localhost:8025/api/events?q=spam'
```

Event history is append-only and has no automatic compaction. Plan disk and
database growth for high-volume deployments; see
[Performance](/operations/performance/).

## Severity conventions

| Severity | Used for |
|----------|----------|
| `debug` | Reserved for future verbose events |
| `info` | Normal lifecycle: connections, stored mail, logins, policy changes |
| `warn` | Rejected recipients, dropped mail, unparsable messages, failed logins |
| `error` | Storage failures, policy engine failures, listener errors |

See [Event reference](/reference/events/) for every type and payload field.
