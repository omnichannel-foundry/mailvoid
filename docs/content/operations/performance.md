---
title: Performance
weight: 70
---

# Performance

MailVoid is designed for development, QA, and low-to-moderate capture volumes.
It is not a high-throughput mail gateway. Understand the cost centers before
sizing a deployment.

## Per-message cost

For one SMTP transaction with `N` recipients, MailVoid performs:

1. One MIME parse.
2. One policy-list load and one regex evaluation per recipient (compiled regexes
   are cached).
3. One database write per recipient (raw message is duplicated per recipient).
4. Several event emissions: connection, MAIL FROM, RCPT TO per recipient, DATA,
   policy match, and message received/dropped.

Each event is:

- appended to `events.jsonl`;
- inserted into the Genji `events` table;
- added to the in-memory ring;
- broadcast to live subscribers (non-blocking).

The file and database sinks are synchronous. A slow disk directly slows SMTP
`DATA` handling. Live slow clients do not slow the server because their
subscription channel is non-blocking; they simply miss events.

## Serialization points

| Component | Behavior |
|-----------|----------|
| Genji writer | One exclusive mutex for all writes (messages, events, policies) |
| Genji reader | Shared RWMutex |
| Event sinks | Called in the emitter goroutine |
| Regex cache | `sync.Map`, compiled once per pattern |
| TTL sweeper | One goroutine; scans for expired messages |

Every SMTP connection runs in its own goroutine, but writes serialize at the
repository. Throughput is bounded by the database write path, not the Go
scheduler.

## Expensive queries

Genji v0.15 does not expose secondary indexes for these access patterns, so the
following scan the relevant table:

- `?q=` filters using `LIKE '%...%'` on `search_text`;
- `DeleteExpired`, which scans messages for a non-zero `expires_at` in range;
- `COUNT(*)` for dashboard and API totals;
- event count for the dashboard.

At small scale these are invisible. At hundreds of thousands of messages, expect
search, counts, and the sweeper to take measurable time.

## Disk and filesystem

- Put the data directory on local SSD/NVMe storage.
- Avoid NFS, SMB, or other network filesystems for the Genji directory; Pebble
  expects reliable local locking and fsync semantics.
- Leave headroom for Pebble compaction and write-ahead logs.
- The JSONL event log grows without bound. Archive it on a schedule.
- Do not run multiple instances against one data directory.

## Memory

Memory is dominated by:

- the Go runtime and Fiber;
- Pebble's block cache and memtables;
- in-flight raw messages (up to `MAILVOID_MAX_MESSAGE_BYTES` each);
- the 256-event ring;
- live subscriber buffers.

A 25 MiB maximum message does not mean each message uses 25 MiB; it is a ceiling.
Concurrent large messages can multiply memory use.

## Full-text search

- Bleve indexes text synchronously after a message is stored. Each accepted
  message therefore adds index write work to the SMTP capture path.
- The index directory grows with message content and compaction headroom.
- Startup may rebuild the index when the document count differs from the store;
  large installations can make the first start after an index loss slow.
- If indexing fails, the message is still stored and a `search.error` event is
  emitted; search can be stale until a rebuild.
- `GET /api/v1/search` and MCP `search_messages` avoid the full message-table
  scan used by the substring fallback for body queries.

## Agent and WebDAV queries

- `GET /api/v1/mailboxes` and the MCP `list_mailboxes` tool aggregate messages by
  scanning the message store; cost grows with total captured mail.
- `GET /api/v1/mailboxes/{address}` uses counts and a one-row lookup.
- `wait_for_email` and `/wait` poll the store every 500 ms for up to 60 seconds.
  Many concurrent waits add read load.
- WebDAV `PROPFIND` on `/dav/` aggregates every mailbox; listing a mailbox pages
  through all of its messages. Large trees make directory listings slower.
- `.eml` downloads stream the stored raw bytes and are cheap relative to
  aggregation.

## HTTP and assets

- The embedded DaisyUI bundle is about 2.9 MiB before HTTP compression. It is
  cached for one hour but is a noticeable first-load cost.
- Server-Sent Events and WebSocket connections consume a goroutine and buffers
  per client.
- The dashboard's count cards run aggregate queries on every page load.

## Capacity planning

There is no universal message-per-second figure; it depends on message size,
recipient count, disk latency, event volume, and policy count. Benchmark with
your own workload:

```bash
# Send 100 small messages and time it.
time for i in $(seq 1 100); do
  swaks --server 127.0.0.1:1025 \
    --from sender@example.org \
    --to inbox@example.test \
    --header "Subject: load $i" \
    --body "small body" >/dev/null
done

curl -s -b cookies.txt http://127.0.0.1:8025/api/stats | jq
```

Watch:

- `iostat` / `nvme-cli` for disk latency;
- free space;
- process RSS;
- Genji directory size;
- event count growth.

## Scaling options

| Need | Approach |
|------|----------|
| More ports/domains on one host | Add ports/domains to the same instance |
| Isolation between projects | Separate MailVoid instances with distinct data dirs and ports |
| More throughput than one instance | Run multiple instances with independent data dirs and route clients |
| Long-term event history | Archive JSONL and export/delete database events externally |
| Lower storage | Use TTL policies aggressively; delete old messages via API |

You cannot share one Genji directory between instances. Horizontal scaling is
partitioning, not clustering.

## Future optimizations

Not implemented in this release:

- asynchronous or batched event sinks;
- event retention/compaction;
- secondary indexes for search and expiry;
- streaming MIME parsing for very large messages;
- compression of embedded static assets;
- per-client SMTP rate limits;
- configurable Pebble cache and write options.

The repository seam and event bus are structured so these can be added without
changing the SMTP or web APIs.
