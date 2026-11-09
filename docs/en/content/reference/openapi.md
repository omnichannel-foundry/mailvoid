---
title: OpenAPI
weight: 15
---

# OpenAPI

MailVoid's canonical machine API is versioned under `/api/v1`. It is described
by an OpenAPI 3.1.0 document embedded in the binary and served at two paths:

| Path | Content type |
|------|--------------|
| `/api/v1/openapi.json` | `application/json` |
| `/api/v1/openapi.yaml` | `application/yaml` |

Both are public so tooling can discover the API without credentials. The
operations themselves require authentication.

## Authentication

The v1 API accepts either:

- `Authorization: Bearer <MAILVOID_API_TOKEN>` for agents and automation;
- the `mailvoid_session` cookie created by `POST /login` for browsers and
  console tooling.

If no API token is configured, bearer requests are rejected and only the session
cookie works. See [Configuration](/getting-started/configuration/).

## Endpoint map

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1` | API metadata and links |
| `GET` | `/api/v1/mailboxes` | Mailbox summaries with counts |
| `GET` | `/api/v1/mailboxes/{address}` | One mailbox summary |
| `GET` | `/api/v1/mailboxes/{address}/messages` | Messages for a mailbox |
| `GET` | `/api/v1/mailboxes/{address}/wait` | Long-poll for a new message |
| `GET` | `/api/v1/messages` | List/filter messages (metadata substring `q`) |
| `GET` | `/api/v1/search` | Ranked Bleve full-text search |
| `GET` | `/api/v1/messages/{id}` | Full message and bodies |
| `GET` | `/api/v1/messages/{id}/eml` | Raw message download |
| `DELETE` | `/api/v1/messages/{id}` | Delete a message |
| `GET` | `/api/v1/events` | Audit events |
| `GET` | `/api/v1/policies` | List policies |
| `POST` | `/api/v1/policies` | Create a policy |
| `GET` | `/api/v1/policies/{id}` | Get a policy |
| `PUT` | `/api/v1/policies/{id}` | Update a policy |
| `DELETE` | `/api/v1/policies/{id}` | Delete a policy |
| `POST` | `/api/v1/policies/reorder` | Reorder policies |
| `POST` | `/api/v1/policies/test` | Test a regex |
| `GET` | `/api/v1/stats` | Capture statistics |
| `GET/POST` | `/api/v1/admin/api-keys` | List/create managed API keys |
| `DELETE` | `/api/v1/admin/api-keys/{id}` | Revoke an API key |
| `GET/POST` | `/api/v1/admin/webdav-users` | List/create WebDAV users |
| `DELETE` | `/api/v1/admin/webdav-users/{username}` | Delete a WebDAV user |

The full request and response schemas, parameters, and security definitions are
in the served document. This page covers the common workflows.

## Check whether mail arrived

```bash
TOKEN='your-api-token'

# All mailboxes
curl -s -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8025/api/v1/mailboxes'

# One mailbox
curl -s -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8025/api/v1/mailboxes/inbox@example.test'

# Recent messages in that mailbox
curl -s -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8025/api/v1/mailboxes/inbox@example.test/messages?limit=10'
```

A mailbox summary includes `message_count`, `latest_message_id`,
`latest_subject`, `latest_sender`, and `latest_received_at`.

## Wait for delivery

Long-poll instead of sleeping in an agent loop:

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8025/api/v1/mailboxes/inbox@example.test/wait?timeout=30s'
```

- `since` is an optional RFC 3339 lower bound. If omitted, the wait starts at the
  current time.
- `timeout` is a Go duration, clamped to 1–60 seconds.
- On timeout the response is `200` with `{"found":false,"waited_seconds":...}`.
- On delivery the response contains `found`, `waited_seconds`, and a message
  summary.

## Search messages

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8025/api/v1/messages?q=invoice&since=2026-11-01T00:00:00Z&limit=50'
```

Supported parameters:

| Parameter | Meaning |
|-----------|---------|
| `q` | Case-insensitive substring over metadata |
| `domain`, `mailbox`, `recipient` | Exact target filters |
| `action` | `keep`, `ttl`, or `drop` |
| `port` | Receiving port |
| `since`, `until` | RFC 3339 bounds |
| `limit` | 1–200, default 50 |
| `page` or `offset` | Pagination; `offset` wins |

Invalid timestamps, ports, actions, limits, or offsets return `400`.

## Download .eml

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  -o message.eml \
  'http://localhost:8025/api/v1/messages/msg_81997d51ccdda1a5/eml'
```

The response is `message/rfc822` with an attachment filename derived from the
subject and message ID.

## Legacy `/api`

The older `/api` paths remain available for the console and existing scripts.
They use the same message, event, policy, and stats handlers, require the session
cookie (no bearer token), and do not include the mailbox, wait, or `.eml`
endpoints. New integrations should use `/api/v1`.

## Tools

Load `/api/v1/openapi.json` into any OpenAPI-aware client, postman collection,
or code generator. The document uses local `$ref` components and contains no
external references.
