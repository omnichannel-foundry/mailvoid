---
title: Lorem mail generator
weight: 55
---

# Lorem mail generator

`lorem-mail-generator` connects to a MailVoid SMTP listener and sends randomized Lorem
Ipsum messages with configurable body word counts. It is useful for exercising
the dashboard, search, pagination, retention policies, and SMTP load handling.

## Build

```bash
make lorem
```

The binary is written to `bin/lorem-mail-generator`.

## Usage

```bash
./bin/lorem-mail-generator -addr 127.0.0.1:1025 -count 100
```

Every message gets a random subject, a generated `Message-ID`, and two test
headers:

- `X-Mailvoid-Lorem-Index`: zero-based message index.
- `X-Mailvoid-Lorem-Words`: requested body word count.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `127.0.0.1:1025` | SMTP server address |
| `-from` | `lorem@example.test` | Envelope sender |
| `-to` | `inbox@example.test` | Comma-separated recipients |
| `-count` | `10` | Number of messages to send |
| `-min-words` | `10` | Minimum body word count |
| `-max-words` | `120` | Maximum body word count |
| `-concurrency` | `1` | Parallel SMTP connections |
| `-delay` | `0` | Delay between messages per worker |
| `-timeout` | `10s` | Per-connection timeout |
| `-seed` | `0` | Random seed; `0` uses the current time |
| `-html` | `false` | Send HTML bodies instead of plain text |
| `-subject-prefix` | `Lorem` | Subject prefix |
| `-fail-fast` | `false` | Stop at the first send error |
| `-quiet` | `false` | Suppress progress output |
| `-random-mailboxes` | `0` | Generate N random recipient mailboxes; overrides `-to` |
| `-mailbox-prefix` | `lorem` | Local-part prefix for random mailboxes |
| `-domains` | `example.test` | Comma-separated domains for random mailboxes |

## Examples

### Volume test

```bash
./bin/lorem-mail-generator \
  -addr 127.0.0.1:1025 \
  -count 500 \
  -min-words 20 \
  -max-words 200 \
  -concurrency 8
```

### Reproducible run

```bash
./bin/lorem-mail-generator -seed 42 -count 25
```

### HTML bodies

```bash
./bin/lorem-mail-generator -html -min-words 50 -max-words 500
```

### Several fixed recipients

```bash
./bin/lorem-mail-generator -to 'inbox@example.test,alerts@example.test' -count 20
```

Each message is delivered to every address in `-to`.

### Random mailboxes

```bash
./bin/lorem-mail-generator \
  -count 500 \
  -random-mailboxes 40 \
  -domains 'example.test,test.local' \
  -mailbox-prefix load
```

This builds a pool of 40 unique addresses such as
`load-dolor-4821@test.local`, then assigns one random mailbox per message.
When `-random-mailboxes` is greater than zero, `-to` is ignored.

## Behavior

- Each message uses a new SMTP connection, so `-concurrency` controls parallel
  connections.
- With `-random-mailboxes`, every message has exactly one recipient chosen from
  the generated pool, which makes mailbox lists and pagination more realistic.
- MailVoid does not implement SMTP authentication or STARTTLS, so this tool does
  not attempt either.
- The command prints a summary with the seed, sent/failed counts, elapsed time,
  and messages per second.
- Exit status is `1` when any message fails, or `2` for invalid flags.
