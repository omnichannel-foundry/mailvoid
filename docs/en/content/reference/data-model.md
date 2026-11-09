---
title: Data model
weight: 40
---

# Data model

MailVoid stores four record types in Genji: messages, policies, events, and
users. The Go types live in `internal/models`; the repository interface lives in
`internal/storage`.

## Messages

### Genji table `messages`

| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | `msg_` + 16 hex characters |
| `received_at` | INT | Unix milliseconds, UTC |
| `remote_ip` | TEXT | TCP peer address |
| `local_port` | INT | Receiving SMTP port |
| `domain` | TEXT | Lower-cased recipient domain |
| `mailbox` | TEXT | Lower-cased local part |
| `recipient` | TEXT | Lower-cased `mailbox@domain` |
| `sender` | TEXT | Lower-cased envelope sender; empty for `<>` |
| `subject` | TEXT | Decoded subject |
| `headers` | TEXT | JSON object: `map[string][]string` |
| `raw_headers` | TEXT | Header block as received |
| `raw` | TEXT | Complete raw message |
| `body_text` | TEXT | Decoded `text/plain` |
| `body_html` | TEXT | Decoded `text/html` |
| `size_bytes` | INT | Raw byte length |
| `policy_id` | TEXT | Matched policy, or empty |
| `policy_name` | TEXT | Matched policy name, or empty |
| `policy_action` | TEXT | `keep`, `ttl`, or `drop` |
| `expires_at` | INT | Unix milliseconds; `0` means no expiry |
| `deleted` | INT | `0`/`1`; reserved. Current deletion is physical. |
| `deleted_at` | INT | Unix milliseconds; reserved |
| `search_text` | TEXT | Lower-cased concatenation used by `?q=` |

### API representation

The JSON API exposes the same content with `snake_case` names. List responses
omit `headers`, `raw_headers`, `raw`, `body_text`, and `body_html`. Detail
responses include all fields. `deleted_at` and policy/expiry fields are omitted
when empty.

### Deletion

Messages are deleted physically. There is no trash or soft-delete workflow; the
`deleted` and `deleted_at` columns are reserved for a future implementation and
remain zero for stored messages.

## Policies

### Genji table `policies`

| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | `pol_` + 16 hex characters |
| `name` | TEXT | Human label |
| `field_name` | TEXT | Target field |
| `pattern` | TEXT | Go RE2 expression |
| `policy_action` | TEXT | `keep`, `ttl`, or `drop` |
| `ttl_seconds` | INT | Required for `ttl`; otherwise 0 |
| `priority` | INT | Ascending evaluation order, starting at 0 |
| `enabled` | INT | `0`/`1` |
| `created_at` | INT | Unix milliseconds |
| `updated_at` | INT | Unix milliseconds |

### API representation

```json
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
```

The API names differ from the storage column names: `field_name` becomes
`field`, and `policy_action` becomes `action`.

## Events

### Genji table `events`

| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | `ev_` + 16 hex characters |
| `event_time` | INT | Unix milliseconds, UTC |
| `event_type` | TEXT | Dotted type |
| `severity` | TEXT | `debug`, `info`, `warn`, `error` |
| `payload` | TEXT | JSON object |
| `search_text` | TEXT | Lower-cased type + severity + payload |

### API representation

```json
{
  "id": "ev_9f3c1d2a7b4e6f80",
  "timestamp": "2026-11-08T01:28:24.083Z",
  "type": "message.received",
  "severity": "info",
  "payload": {"message_id": "msg_81997d51ccdda1a5"}
}
```

Storage column names are `event_time` and `event_type`; the JSON names are
`timestamp` and `type`.

## Users

### Genji table `users`

| Column | Type | Notes |
|--------|------|-------|
| `username` | TEXT PK | Lower-cased |
| `password_hash` | TEXT | bcrypt hash (cost 12) |
| `created_at` | INT | Unix milliseconds |

Passwords are never returned by the API and are not present in event payloads.
The shipped application exposes a single bootstrap account and a password-change
flow rather than user administration endpoints.

## Identifiers

| Prefix | Record |
|--------|--------|
| `msg_` | Message |
| `pol_` | Policy |
| `ev_` | Event |

Identifiers are generated with `crypto/rand` and hex-encoded. They are
URL-safe and case-sensitive.

## Time handling

- All persisted times are UTC.
- Storage uses Unix milliseconds; `0` represents "absent" for optional times
  (`expires_at`, `deleted_at`).
- JSON uses RFC 3339 with a `Z` suffix.
- The console displays UTC timestamps; the live event client formats the arrival
  time in the browser's local timezone.

## Search text

`search_text` is a storage-only convenience column:

- message: lower-cased sender, recipient, mailbox, domain, subject, and remote IP;
- event: lower-cased type, severity, and serialized payload.

Filters lowercase the user query and use `LIKE '%query%'` against this column.
Search does not include message bodies, raw headers, or the raw message.
