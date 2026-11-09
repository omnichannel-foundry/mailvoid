---
title: Events
weight: 30
---

# Events

Every event has the same envelope:

```json
{
  "id": "ev_9f3c1d2a7b4e6f80",
  "timestamp": "2026-11-08T01:28:24.083Z",
  "type": "smtp.rcpt_to",
  "severity": "info",
  "payload": {"accepted": true}
}
```

`payload` is omitted when empty. `timestamp` is UTC. The JSONL file stores one
compact object per line in emission order. Genji stores the same fields plus a
lower-cased `search_text` used by `?q=` filtering.

## Severities

| Severity | Meaning |
|----------|---------|
| `debug` | Reserved; no built-in events currently use it |
| `info` | Normal lifecycle |
| `warn` | Rejections, drops, parse failures, failed logins |
| `error` | Storage/listener/policy failures |

## System

| Type | Severity | Payload |
|------|----------|---------|
| `system.startup` | info | `version`, `smtp_ports` (array), `smtp_addresses` (array), `smtp_services` (array), `hostname`, `scheme`, `public_url`, `event_log_enabled`, `capture_enabled`, `log_enabled`, `log_dir`, `http_addr`, `domains` (array), `data_dir`, `db_path`, `event_log`, `admin_user`, `started_at` (RFC 3339), `max_message` (bytes) |
| `system.capture` | info | `enabled` (bool), `username`, `remote_ip` |
| `system.shutdown` | info | `version`, `reason` (`"shutdown"`) |

## SMTP lifecycle

The JSONL stream is the cross-service audit trail. Each SMTP service also writes
a separate traditional text log (see [Configuration](/getting-started/configuration/)).

| Type | Severity | Payload |
|------|----------|---------|
| `smtp.connected` | info | `remote_ip`, `local_port`, `service`, `aliases` (array), `hostname` (client EHLO/HELO name) |
| `smtp.mail_from` | info | `remote_ip`, `local_port`, `sender` |
| `smtp.rcpt_to` | info / warn | `remote_ip`, `local_port`, `recipient`, `domain`, `mailbox`, `accepted` (bool). Rejections add `reason`. |
| `smtp.data` | info | `remote_ip`, `local_port`, `size` (bytes), `recipients` (count) |
| `smtp.closed` | info | `remote_ip`, `local_port` |
| `smtp.error` | warn / error | Context-dependent; see below |

### `smtp.rcpt_to` rejection payloads

Malformed address:

```json
{
  "remote_ip": "203.0.113.9",
  "local_port": 1025,
  "recipient": "not-an-address",
  "accepted": false,
  "reason": "invalid address"
}
```

Domain not allowed:

```json
{
  "remote_ip": "203.0.113.9",
  "local_port": 1025,
  "recipient": "user@denied.test",
  "domain": "denied.test",
  "accepted": false,
  "reason": "domain not accepted"
}
```

### `smtp.error` payloads

| Situation | Fields |
|-----------|--------|
| Listener failure | `error`, `local_port` |
| Message too large | `remote_ip`, `local_port`, `error` (`"message too large"`), `size` |
| MIME parse failure (message still stored) | `remote_ip`, `local_port`, `error`, `stored:true` |
| Policy evaluation failure (message kept) | `remote_ip`, `local_port`, `recipient`, `error` |

## Messages

| Type | Severity | Payload |
|------|----------|---------|
| `message.received` | info | Message metadata |
| `message.dropped` | warn | Message metadata + `reason:"policy drop"` |
| `message.deleted` | info | Message metadata + `reason` + actor fields |
| `message.error` | error | Failure-specific |

### Message metadata fields

`message.received` and `message.dropped` include:

| Field | Description |
|-------|-------------|
| `message_id` | Stored message ID |
| `remote_ip` | Source IP |
| `local_port` | Receiving port |
| `domain` | Recipient domain |
| `mailbox` | Recipient local part |
| `recipient` | Full recipient |
| `sender` | Envelope sender |
| `subject` | Decoded subject |
| `size` | Raw size in bytes |
| `action` | `keep`, `ttl`, or `drop` |
| `policy_id` | Matched policy ID, or empty |
| `policy` | Matched policy name, or empty |
| `expires_at` | RFC 3339 timestamp for TTL messages |
| `reason` | Present for drops (`"policy drop"`) |

### `message.deleted` variants

| Source | Additional fields |
|--------|-------------------|
| TTL sweeper | `reason:"ttl expiry"`, `policy` (matched name) |
| Console delete | `reason:"manual deletion"`, `username` |
| API delete | `reason:"api deletion"`, `username` |
| MCP delete | `reason:"mcp deletion"`, `username:"mcp"` |

All variants include `message_id`, `recipient`, `mailbox`, `domain`,
`local_port`, `remote_ip`, `subject`, and `action`.

### `message.error`

| Situation | Fields |
|-----------|--------|
| SMTP storage failure | `remote_ip`, `local_port`, `recipient`, `error`; the client receives `451` |
| TTL sweep failure | `error`, `reason:"ttl sweep"` |

## Policies

| Type | Severity | Payload |
|------|----------|---------|
| `policy.matched` | info | `remote_ip`, `local_port`, `recipient`, `policy_id`, `policy_name`, `field`, `action` |
| `policy.created` | info | `policy_id`, `name`, `field`, `pattern`, `action`, `ttl_seconds`, `enabled`, `priority`, `username` |
| `policy.updated` | info | Same as created (used for edits, toggles, and API updates) |
| `policy.deleted` | info | Same as created |
| `policy.reordered` | info | Console: `policy_id`, `direction` (`up`/`down`), `username`; API: `ids` (ordered array), `username` |

`policy.matched` is emitted once per recipient when a rule matches. It does not
include the matched text value.

## Credentials

| Type | Severity | Payload |
|------|----------|---------|
| `api_key.created` | info | `key_id`, `name`, `prefix`, `expires_at`, `username` |
| `api_key.deleted` | info | `key_id`, `name`, `prefix`, `username` |
| `webdav_user.created` | info | `username`, `generated`, `username_actor` |
| `webdav_user.deleted` | info | `username`, `username_actor` |

Tokens and passwords are never included.

## Search

| Type | Severity | Payload |
|------|----------|---------|
| `search.error` | error | `error` |

A search indexing failure does not fail message capture; the message remains in
Genji and the index may be stale until a rebuild.

## Authentication

| Type | Severity | Payload |
|------|----------|---------|
| `auth.login.success` | info | `username`, `remote_ip`, `user_agent` |
| `auth.login.failed` | warn | `username` (attempted), `remote_ip`, `user_agent` |
| `auth.logout` | info | `username`, `remote_ip` |
| `auth.password.changed` | info | `username`, `remote_ip` |

Passwords are never included in events.

## Ordering and retention

- Genji and API listings return events newest first by timestamp.
- The JSONL file preserves emission order.
- The in-memory ring keeps the last 256 events.
- There is no event retention or compaction. The table and JSONL file grow for
  the lifetime of the deployment unless you archive them externally.
