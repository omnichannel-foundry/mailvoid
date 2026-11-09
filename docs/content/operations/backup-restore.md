---
title: Backup and restore
weight: 40
---

# Backup and restore

MailVoid has two pieces of state:

1. `<data-dir>/mailvoid.db/` — the Genji/Pebble database directory.
2. `<data-dir>/events.jsonl` — the append-only JSONL event log.
3. `<data-dir>/search.bleve/` — the Bleve full-text index.

The search index is rebuildable from Genji; you may exclude it to save backup
space and time. It contains searchable message terms, so if you do back it up,
apply the same encryption and access controls as the database.

## Safe backup

Pebble uses a write-ahead log. Do not copy the database directory while the
process is writing and expect a valid snapshot. Choose one:

### Stop, copy, start

```bash
sudo systemctl stop mailvoid
sudo tar -C /var/lib -czf /backup/mailvoid-$(date +%F).tgz mailvoid
sudo systemctl start mailvoid
```

### Filesystem snapshot

If your filesystem supports atomic snapshots (LVM, ZFS, btrfs), snapshot the
volume while the process is running. The snapshot is consistent even though the
live directory is not safe to copy directly.

### API export

For a logical export without stopping the service:

```bash
curl -s -b cookies.txt 'http://127.0.0.1:8025/api/messages?limit=200' > messages.json
```

This is paginated metadata only; use the detail endpoint per message if you need
raw content. It is not a full-fidelity backup of policies, users, or events.

## Restore

```bash
sudo systemctl stop mailvoid
sudo mv /var/lib/mailvoid /var/lib/mailvoid.old
sudo tar -C /var/lib -xzf /backup/mailvoid-2026-11-08.tgz
sudo chown -R mailvoid:mailvoid /var/lib/mailvoid
sudo chmod 0750 /var/lib/mailvoid
sudo systemctl start mailvoid
```

Verify:

```bash
curl -s http://127.0.0.1:8025/healthz
curl -s -b cookies.txt http://127.0.0.1:8025/api/stats
```

If the administrator password or session secret changes, existing browser
sessions are invalid; sign in again.

## Event log rotation

The JSONL file is append-only and independent of the database. When the service
is stopped, you can archive or truncate it without affecting messages or
policies:

```bash
sudo systemctl stop mailvoid
sudo mv /var/lib/mailvoid/events.jsonl /backup/events-$(date +%F).jsonl
sudo systemctl start mailvoid
```

Genji's `events` table retains its own history. Deleting the JSONL file does not
delete events from the API or console.

## What is not backed up

- The session store is in memory; sessions are intentionally ephemeral.
- No external object storage, mail queue, or caches exist.
- Build artifacts and embedded assets are reproducible from the binary.

## Validation

After a restore, check:

- the database opens without a lock error;
- the message count matches expectations;
- policies appear in the correct order;
- events are present in the console and `events.jsonl`;
- TTL messages have sensible expiry times (absolute timestamps are preserved, so
  restoring an old backup may expire messages immediately).
