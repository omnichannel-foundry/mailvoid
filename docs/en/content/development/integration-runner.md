---
title: Integration runner
weight: 50
---

# Integration runner

`mailvoid-it` is a standalone Go binary that launches MailVoid and exercises
the REST API, MCP tools, SMTP capture, managed credentials, `.eml` downloads,
WebDAV, and full-text search.

It has two modes:

- **Interactive** — a Charmbracelet Bubble Tea TUI for selecting and running
  tests.
- **Batch** — a CI-friendly runner driven by JTD-validated JSON files.

The runner is independent of the application packages: it talks over HTTP, SMTP,
MCP, and WebDAV only.

## Build and run

```bash
make it                 # build bin/mailvoid-it
make integration        # build both, run the full suite in batch mode
make integration-tui    # open the interactive runner
```

The default suite lives in `testdata/integration`:

```text
testdata/integration/
├── schema.jtd.json
├── 01-credentials.json
├── 02-mail-flow.json
└── 03-search.json
```

`make integration` launches `bin/mailvoid` with a temporary data directory,
free local ports, search enabled, no bootstrap API/WebDAV credentials, and the
administrator password `mailvoid-it-password` unless overridden.

## CLI

| Flag | Default | Meaning |
|------|---------|---------|
| `-suite` | `testdata/integration` | Directory containing JTD tests |
| `-server` | `bin/mailvoid` | Server binary to launch |
| `-base-url` | *(empty)* | Use an external HTTP endpoint instead of launching |
| `-smtp-addr` | *(empty)* | External SMTP `host:port`; required with `-base-url` |
| `-admin-user` | `admin` | Administrator username |
| `-admin-password` | `mailvoid-it-password` | Administrator password |
| `-batch` | `false` | Disable the TUI |
| `-validate-only` | `false` | Validate JTD files and exit |
| `-list` | `false` | List tests and exit |
| `-test` | *(empty)* | Comma-separated name/file filter |
| `-verbose` | `false` | Show all step logs in batch mode |
| `-keep` | `false` | Keep the launched server's data directory |

When stdout is not a terminal, the runner automatically uses batch mode.

## Interactive TUI

Keys:

| Key | Action |
|-----|--------|
| `↑` / `↓` or `j` / `k` | Move selection |
| `space` | Toggle a test |
| `a` | Select all / none |
| `enter` | Run selected tests (or rerun from the results view) |
| `q` / `ctrl-c` | Quit |

The TUI shows:

- server URL and temporary data directory;
- per-test selection and pass/fail state;
- a spinner and completed/total counter while running;
- `✔` / `✖` status glyphs with semantic colors;
- a scrollable step log panel for the highlighted test.

It uses `bubbletea`, `lipgloss`, `bubbles/spinner`, and `bubbles/viewport` from
the Charmbracelet ecosystem. Batch output falls back to plain labels when stdout
is not a TTY or `NO_COLOR` is set.

## JTD test files

Every test file is a JSON document validated against `schema.jtd.json` (JSON
Type Definition, RFC 8927) before execution. A file contains a name, optional
description, and an ordered list of steps.

```json
{
  "name": "mail delivery surfaces",
  "description": "Send SMTP and verify delivery over REST, MCP, and WebDAV.",
  "steps": [
    {
      "type": "smtp",
      "name": "send mail",
      "from": "sender@example.org",
      "to": ["inbox-${run_id}@example.test"],
      "subject": "Delivery ${run_id}",
      "text": "unique body ${run_id}",
      "port": "${smtp_port}"
    },
    {
      "type": "http",
      "name": "check mailbox",
      "method": "GET",
      "path": "/api/v1/mailboxes/inbox-${run_id}@example.test",
      "bearer": "${api_key}",
      "save": { "message_id": "latest_message_id" },
      "expect": {
        "status": 200,
        "json": [{ "path": "message_count", "equals": 1 }]
      }
    }
  ]
}
```

### Step types

| Type | Required fields | Notes |
|------|-----------------|-------|
| `http` | `method`, `path` | JSON `body` or `body_raw`; `auth: session`, `bearer`, or `basic` |
| `webdav` | `method`, `path`, `basic` | `depth`, XML `body_raw` |
| `mcp` | `tool` | `arguments`; `bearer` or session auth |
| `smtp` | `from`, `to` | `subject`, `text`, `html`, `port` |
| `sleep` | `millis` | Pause between steps |

### Variables

Built-ins: `${run_id}`, `${base_url}`, `${smtp_host}`, `${smtp_port}`,
`${smtp_port2}`, `${admin_user}`, `${admin_password}`, `${domain}`.

The `save` map stores values from HTTP JSON or MCP structured results:

```json
"save": { "api_key": "token", "key_id": "id", "message_id": "messages.0.id" }
```

Path segments are dot-separated; numeric segments index arrays. Undefined
variables or missing save paths fail the step.

### Assertions

`expect` supports:

| Field | Meaning |
|-------|---------|
| `status` | Exact HTTP status |
| `body_contains`, `body_not_contains` | Raw body substrings |
| `headers` | Header substring checks |
| `json` | A list of path assertions |

Each JSON assertion can use `exists`, `equals`, `contains`, `gte`, `lte`, and
`length`. Assertions run against parsed HTTP JSON or MCP structured output.

## Isolation and cleanup

Each test gets a fresh administrator cookie jar and its own `run_id`. Use the
run ID in mailbox names, API key names, and WebDAV usernames. The tests delete
their credentials and messages; the launched server uses a temporary data
directory that is removed unless `-keep` is set.

## Adding a test

1. Add `testdata/integration/NN-name.json`.
2. Validate shapes with `mailvoid-it -validate-only`.
3. Run one test with `-test name` in batch or select it in the TUI.
4. Prefer `${run_id}`-scoped addresses so tests remain independent.
5. Never put real secrets in the JSON; create credentials through the admin
   session and save the returned token into a variable.
