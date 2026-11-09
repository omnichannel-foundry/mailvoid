---
title: Full-text search
weight: 24
---

# Full-text search

MailVoid uses an embedded **Bleve** index as a rebuildable projection of the
Genji message store. It provides tokenized, ranked (BM25) search over message
content while keeping the authoritative raw bytes in Genji.

## Configuration

| Variable | Default | Meaning |
|----------|---------|---------|
| `MAILVOID_SEARCH_ENABLED` | `true` | Enable the Bleve index |
| `MAILVOID_SEARCH_INDEX` | `<data-dir>/search.bleve` | Index directory |

Flags: `-search-enabled`, `-search-index`.

The index is opened at startup. If it is missing or its document count differs
from the message store, it is rebuilt from Genji. The index is not
authoritative: deleting it and restarting is safe.

## Indexed fields

- subject;
- decoded plain-text body;
- HTML body with tags stripped;
- sender and recipient;
- mailbox, domain, and source IP;
- raw headers;
- action, receiving port, size, received timestamp.

The complete raw message is **not** indexed; retrieve it with `get_message` or
the `.eml` endpoint after finding a hit. Attachments are not extracted or
indexed separately.

Keyword fields mirror mailbox, domain, recipient, sender, action, and source IP
so filters can be combined with the text query. The REST and MCP surfaces use
those structured filters rather than a raw query language.

## REST

`GET /api/v1/search`:

```bash
curl -s -H "Authorization: Bearer $MAILVOID_API_TOKEN" \
  'http://localhost:8025/api/v1/search?q=invoice&mailbox=inbox&domain=example.test&limit=20'
```

| Parameter | Meaning |
|-----------|---------|
| `q` | Full-text query |
| `mailbox`, `domain`, `recipient`, `sender` | Exact metadata filters |
| `action` | `keep`, `ttl`, or `drop` |
| `port` | Receiving port |
| `since`, `until` | RFC 3339 bounds |
| `limit`, `offset`, `page` | Pagination |
| `sort` | `relevance` (default) or `received_at` |

Response:

```json
{
  "messages": [
    {
      "id": "msg_...",
      "received_at": "2026-11-08T05:00:00Z",
      "mailbox": "inbox",
      "domain": "example.test",
      "sender": "billing@example.org",
      "subject": "Invoice 42",
      "action": "keep"
    }
  ],
  "count": 1,
  "limit": 20,
  "offset": 0,
  "backend": "bleve",
  "highlights": {
    "msg_...": {
      "subject": ["<mark>Invoice</mark> 42"],
      "body": ["Your <mark>invoice</mark> is attached"]
    }
  }
}
```

`backend` is `bleve` when the index is active and `fallback` when the index is
disabled or unavailable. The fallback preserves the legacy metadata substring
search and does not search bodies.

> [!NOTE]
> `GET /api/v1/messages?q=...` keeps its original metadata substring behavior.
> Use `/api/v1/search` for body search and ranking.

## MCP

The `search_messages` tool uses the same index and filters:

```json
{
  "q": "invoice",
  "mailbox": "inbox",
  "domain": "example.test",
  "limit": 20,
  "sort": "relevance"
}
```

The result includes `messages`, `count`, `backend`, and optional `highlights`.

## Index maintenance

- Messages are indexed synchronously after a successful store write through an
  `IndexedStore` decorator. SMTP capture, API/console deletes, and TTL expiry
  all update the index.
- If indexing fails, the message is still stored and a `search.error` event is
  emitted. Search may be stale until a rebuild.
- The index is rebuilt on startup when its document count differs from the
  store.
- The Bleve index directory contains terms and stored searchable fields. Treat
  it as sensitive data and include it in the same access controls as the
  database. It is rebuildable, so backups may omit it.

## Tradeoffs

Bleve v2.5.7 is the newest release compatible with Go 1.24; v2.6+ requires Go
1.25. It is pure Go and builds with `CGO_ENABLED=0`, at the cost of a larger
binary and a second on-disk structure alongside Genji.
