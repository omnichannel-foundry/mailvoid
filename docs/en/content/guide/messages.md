---
title: Messages
weight: 20
---

# Messages

## Listing

The **Messages** page lists captured mail newest first. Each row shows:

- subject and envelope sender;
- target mailbox and domain;
- receiving port;
- source IP;
- policy action (`keep`, `ttl`, or `drop` — dropped messages never appear);
- received timestamp.

Filters use HTMX for in-place updates:

| Filter | Behavior |
|--------|----------|
| Search | Case-insensitive substring match over sender, recipient, mailbox, domain, subject, and remote IP. For ranked body search use [Full-text search](/reference/search/). |
| Domain | Exact, case-insensitive domain match |
| Mailbox | Exact, case-insensitive local-part match |
| Port | Exact receiving port |
| Action | `keep` or `ttl` (dropped messages are not stored) |

Pagination defaults to 50 rows per page and caps at 200. The JSON API accepts
the same filters.

## Detail view

Open a message to see:

### Envelope card

Target mailbox, source IP, destination port, receive time, size, message ID,
matched policy, and expiry.

### HTML preview

If the message has an HTML body, it is rendered inside a sandboxed `<iframe>`:

```html
<iframe sandbox referrerpolicy="no-referrer" srcdoc="..."></iframe>
```

The sandbox has no `allow-scripts` or `allow-same-origin`, so scripts in the
email do not execute in the MailVoid origin. Remote resources may still load
inside the isolated frame; use network isolation if that matters.

### Plain-text body

The decoded `text/plain` part, with a copy button.

### Headers

A parsed header table plus the raw header block, with a copy button.

### Raw message

The exact bytes that arrived, with a copy button.

## Expiry countdown

Messages with a TTL policy show a live countdown. The server renders an RFC 3339
timestamp in `data-expires`; the client updates the text every second. When the
timer reaches zero the sweeper removes the message at the next sweep tick, and
the page may still show the message until it is reloaded.

The sweeper runs every `MAILVOID_SWEEP_INTERVAL` (default `30s`). A message does
not disappear at the exact second its TTL elapses; it disappears at the next
sweep after expiry.

## Deleting

Use the **Delete** button on the detail page or the API:

```bash
curl -s -b cookies.txt -X DELETE \
  http://localhost:8025/api/messages/<message-id>
```

Deletion is immediate and permanent. It emits `message.deleted` with:

- `reason`: `manual deletion` or `api deletion`;
- `username`: the authenticated account;
- message metadata for the audit trail.

TTL deletions use `reason: "ttl expiry"` and no username.

## Downloading .eml

Use **Download .eml** on the detail page, or the versioned API:

```bash
curl -s -b cookies.txt -o message.eml \
  http://localhost:8025/api/v1/messages/<message-id>/eml
```

WebDAV clients can fetch the same bytes from
`/dav/<mailbox>@<domain>/<message-id>.eml`. See [WebDAV](/operations/webdav/).

## API access

```bash
# list
curl -s -b cookies.txt \
  'http://localhost:8025/api/messages?q=invoice&port=1025&limit=20'

# detail
curl -s -b cookies.txt \
  http://localhost:8025/api/messages/<message-id>
```

The list returns metadata only. The detail endpoint returns the full record,
including headers, raw headers, raw message, and decoded bodies.

## Storage format

Each message is a row in the Genji `messages` table and a JSON object in API
responses. Headers are stored as a JSON object of string arrays. Times are
stored as Unix milliseconds and rendered as RFC 3339 in JSON. See
[Data model](/reference/data-model/) for the complete schema.
