---
title: Extending MailVoid
weight: 40
---

# Extending MailVoid

The code is organized so common extensions touch a small number of files.

## Add an event type

1. Add a constant in `internal/events/event.go`.
2. Emit it with `bus.Emit(events.New(events.TypeX, events.SeverityInfo, payload))`.
3. Add it to `eventTypes` in `internal/web/handlers_pages.go` so it appears in the
   console filter.
4. Update [the event reference](/reference/events/).
5. Add a test asserting the event is emitted.

Keep payload keys stable: they are part of the public API and audit contract.

## Add a policy field

1. Add a constant to `internal/models/models.go`.
2. Append it to `PolicyFields` so validation accepts it and the UI select
   renders it.
3. Add a case to `policy.FieldValue` in `internal/policy/engine.go`.
4. If it belongs in the `any` concatenation, add it there too.
5. Add tests for matching and validation.
6. Update the policies guide and API reference.

## Add a message column

1. Add the field to `models.Message` and its JSON tag.
2. Add it to `dbMessage` in `internal/storage/genji.go`.
3. Add the column to the `CREATE TABLE` statement and to `messageColumns`.
4. Include it in `messageToDB` and `dbMessage.model()`.
5. Update the INSERT and UPDATE statements and argument lists.
6. Update the memory store clone if the field is a pointer or reference type.
7. Consider `search_text` if the field should be searchable.
8. Update the API summary if it belongs in list responses.
9. Add storage round-trip tests.
10. Back up and migrate existing databases; there is no automatic migration.

## Add a store method

1. Add the method to `storage.Store`.
2. Implement it in `storage.Memory`.
3. Implement it in `storage.Genji` within the Genji SQL subset.
4. Add assertions to the shared suite in `store_test.go`; it runs against both.
5. Check concurrency: reads use `RLock`, writes use `Lock`.
6. Avoid multi-column `ORDER BY`; sort secondary keys in Go.

## Add a configuration option

1. Add a field to `config.Config`.
2. Set a default in `Default()` or `finalize()`.
3. Read `MAILVOID_*` in `applyEnv`.
4. Add a flag in `applyFlags`.
5. Validate it if invalid values are possible.
6. Add a config test.
7. Document it in [Configuration](/getting-started/configuration/) and
   [CLI and flags](/reference/cli/).

## Add an API endpoint

1. Register the route in `internal/web/server.go`.
2. Implement the handler in `handlers_api.go`.
3. Parse input with Fiber's `BodyParser` or query helpers.
4. Return errors through `apiError`.
5. Emit an event if the operation changes state.
6. Add a test using the cookie helper in `server_test.go`.
7. Document the request and response in the API reference.

## Add a page or fragment

1. Create the template under `templates/pages` or `templates/fragments`.
2. If it is a page, add it to `pageLayouts` in `render.go` and add a route and
   handler.
3. Use `s.render` for pages and `s.renderFragment` for HTMX responses.
4. Add the nav item to `partials.html` and update `activeFor`/`titleFor`.
5. Keep user content escaped; use a sandboxed iframe for email HTML.

## Add an event sink

Implement `events.Sink`:

```go
type Sink interface {
    Write(Event) error
}
```

Pass it to `events.NewBus(...)` in `cmd/mailvoid/main.go`. Sinks are called
synchronously; keep them fast or buffer internally. A blocking sink slows SMTP.

## Replace the database

Implement `storage.Store` and wire it in `main.go`. Nothing above `storage`
imports Genji types. Run the shared repository suite. See
[Storage](/reference/storage/).

## Schema migrations

The application creates tables with `CREATE TABLE IF NOT EXISTS` and does not
carry a migration framework. If you add columns:

- write an explicit migration for existing databases (or document a
  recreate-and-import path);
- back up first;
- verify both fresh and upgraded databases with tests.

## Code conventions

- No comments that restate the code.
- Document package contracts and exported identifiers.
- Keep persistence behind `storage.Store`.
- Emit events for state changes.
- Prefer stdlib plus the pinned dependencies; do not add an external runtime
  service.
- Add tests in the package you change; use the shared store suite for storage
  changes.
