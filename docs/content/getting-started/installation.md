---
title: Installation
weight: 10
---

# Installation

## Prerequisites

- **Go 1.24.2 or newer** to build from source.
- **GNU Make** (optional, recommended).
- **Hugo 0.158 or newer** only if you want to build this documentation site.
- No CGO, Node.js, npm, or external database is required for the application.

The module pins dependencies that build on Go 1.24.2:

- `github.com/genjidb/genji v0.15.3`
- `github.com/gofiber/fiber/v2 v2.52.16`
- `github.com/emersion/go-smtp v0.25.1`
- `github.com/gofiber/websocket/v2 v2.2.1`
- `github.com/blevesearch/bleve/v2 v2.5.7`
- `golang.org/x/crypto v0.42.0`
- `golang.org/x/net v0.45.0`
- `github.com/modelcontextprotocol/go-sdk v1.4.0`

## Build with Make

```bash
git clone <your-repository-url>
cd mailvoid
make build
./bin/mailvoid --version
```

`make build` injects version, commit, and build date through `-ldflags` and
writes `bin/mailvoid`.

For a stripped, statically linked release binary:

```bash
make release
```

This sets `CGO_ENABLED=0` and adds `-s -w`, producing a smaller binary.

## Build without Make

```bash
go build -trimpath -o mailvoid ./cmd/mailvoid
```

## Install

```bash
make install                            # ~/.local/bin/mailvoid
sudo make install PREFIX=/usr/local     # /usr/local/bin/mailvoid
```

The default install location is `$(PREFIX)/bin/mailvoid` with
`PREFIX="$HOME/.local"`. Use `DESTDIR` for staged packaging.

## First run

```bash
MAILVOID_ADMIN_PASSWORD='choose-a-strong-password' ./bin/mailvoid
```

MailVoid creates `./data`, opens `./data/mailvoid.db`, and appends to
`./data/events.jsonl`. It then logs the SMTP listeners and the dashboard
address:

```text
mailvoid created administrator "admin" from configured credentials
mailvoid SMTP listening on [::]:1025
mailvoid SMTP listening on [::]:2525
mailvoid HTTP dashboard listening on :8025
```

If you omit `MAILVOID_ADMIN_PASSWORD`, a random password is generated on first
start and printed to the log exactly once:

```text
mailvoid created administrator "admin" with generated password: ...
```

Store that password and change it in **Settings** after signing in.

## Verify

```bash
curl -s http://localhost:8025/healthz
# {"status":"ok","version":"...","uptime":"..."}
```

## Upgrade

1. Stop the old process.
2. Replace the binary.
3. Start it with the same data directory.

The schema is created with `CREATE TABLE IF NOT EXISTS`. There is no general
schema migration framework; adding new columns to an existing database requires
a deliberate migration step. Back up the data directory before upgrading across
versions that change the schema.

## Uninstall

```bash
make uninstall
```

Remove the binary only; the data directory is never deleted by the Makefile.
Back it up first if it contains messages you need.
