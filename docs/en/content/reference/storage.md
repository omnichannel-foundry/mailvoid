---
title: Storage
weight: 50
---

# Storage

## The repository interface

Every persistence operation goes through `internal/storage.Store`:

```go
type Store interface {
    events.Sink

    SaveMessage(ctx context.Context, m *models.Message) error
    GetMessage(ctx context.Context, id string) (*models.Message, error)
    ListMessages(ctx context.Context, f models.MessageFilter) ([]*models.Message, error)
    CountMessages(ctx context.Context, f models.MessageFilter) (int, error)
    DeleteMessage(ctx context.Context, id string) error
    DeleteExpired(ctx context.Context, now time.Time) ([]*models.Message, error)

    SavePolicy(ctx context.Context, p *models.Policy) error
    GetPolicy(ctx context.Context, id string) (*models.Policy, error)
    ListPolicies(ctx context.Context) ([]*models.Policy, error)
    DeletePolicy(ctx context.Context, id string) error
    ReorderPolicies(ctx context.Context, ids []string) error

    SaveEvent(ctx context.Context, ev events.Event) error
    ListEvents(ctx context.Context, f models.EventFilter) ([]events.Event, error)
    CountEvents(ctx context.Context, f models.EventFilter) (int, error)

    GetUser(ctx context.Context, username string) (*models.User, error)
    SaveUser(ctx context.Context, u *models.User) error

    Close() error
}
```

The store is also an `events.Sink` (`Write(Event) error`), so the event bus can
persist events without knowing whether the backing store is memory or Genji.

## Implementations

| Implementation | Use |
|----------------|-----|
| `storage.Genji` | Production, persistent |
| `storage.Memory` | Tests and ephemeral use |

`storage.Memory` is fully functional for capture and policy evaluation. It is
used by the test suite and is a convenient choice if you embed MailVoid in a
larger application that supplies its own persistence.

## Genji

### Version and rationale

MailVoid pins **Genji v0.15.3**, the last release published under
`github.com/genjidb/genji`. Later releases declare the module path
`github.com/chaisql/chai`, and v0.17+ require Go 1.25. Upstream development moved
to Chai, but v0.15.3 builds cleanly on Go 1.24 and supports the query shapes
MailVoid needs.

### Database location

`genji.Open(<db-path>)` creates a **directory** at the configured path. It is a
Pebble-backed store containing:

```text
data/mailvoid.db/
├── 000002.log
├── CURRENT
├── LOCK
├── MANIFEST-000001
└── OPTIONS-000003
```

A `LOCK` file prevents two processes from opening the same directory. Run only
one MailVoid instance per data directory.

### Schema creation

On startup the repository executes `CREATE TABLE IF NOT EXISTS` for:

- `messages`
- `policies`
- `events`
- `users`

There is no migration framework. Adding columns to an existing database requires
a deliberate migration. Back up the data directory before upgrading across
schema-changing releases.

### Concurrency

- Writes take an exclusive mutex.
- Reads take a shared RWMutex.
- Event sink writes go through the same lock.
- The race detector passes for the shared store test suite.

### Genji SQL notes

The repository deliberately stays within a small SQL subset:

- `LIKE`, `IN`, `LIMIT`, `OFFSET`, `COUNT(*)`, and positional `?` arguments;
- a single `ORDER BY` expression per query (secondary ordering is applied in
  Go);
- `LOWER()` is unavailable, so case-insensitive search uses the pre-lowered
  `search_text` column;
- no native upsert: inserts fall back to updates when
  `genji.IsAlreadyExistsError` is true.

## In-memory store

`storage.Memory` keeps maps behind an RWMutex:

- messages are cloned on read and write;
- list queries filter and sort copies outside the lock;
- events accumulate in a slice;
- no persistence across restarts.

## Filtering semantics

Both implementations share the same filter behavior:

- `Query` is a case-insensitive substring over `search_text`.
- `Domain`, `Mailbox`, and `Action` are exact matches.
- `Port` is exact.
- `Since` and `Until` are inclusive bounds on received time.
- `Limit` defaults to 50 and caps at 500 in the repository. HTTP handlers cap at
  200.
- `Offset` is zero-based.

Message listings are newest first. Event listings are newest first.

## Backup

The database is a directory and the event log is a file. Stop the process and
copy both together. See [Backup and restore](/operations/backup-restore/).

## Replacing Genji

To move to another embedded database:

1. Implement `storage.Store` in a new package.
2. Wire it in `cmd/mailvoid/main.go`.
3. Keep the `CreateTable` semantics out of other packages; nothing above
   `storage` depends on Genji types.
4. Run the shared repository tests from `internal/storage/store_test.go`.

The event bus, SMTP, policy, and web layers require no changes.
