---
title: Testing
weight: 20
---

# Testing

## Commands

```bash
make test        # all packages
make test-race   # race detector, uncached
make cover       # coverage.out + HTML report
make check       # fmt-check + vet + test
```

The usual Go commands work too:

```bash
go test ./...
go test -race -count=1 ./internal/storage
go test -run TestPolicy ./internal/policy -v
```

## Test layers

| Package | What is covered |
|---------|-----------------|
| `internal/events` | Bus fan-out, history ring, unsubscribe, JSONL sink |
| `internal/storage` | Shared suite run against memory and Genji: round trips, filters, pagination, expiry, policy ordering, events, users |
| `internal/policy` | First-match wins, all fields, TTL expiries, disabled/invalid rules, validation, regex tester |
| `internal/mailparse` | Plain text, encoded subjects, multipart/base64, quoted-printable/Latin-1, HTML-only, attachments, malformed input |
| `internal/auth` | Hash/verify, case-insensitive user lookup, bootstrap idempotence, reset, generated passwords |
| `internal/config` | Defaults, env/flags precedence, port parsing, domain wildcards, invalid values, custom paths |
| `internal/smtpserver` | Real SMTP client against ephemeral listeners: capture, multiple ports, domain rejection, drop/TTL policies, multiple recipients, unparsable messages, oversized mail, storage/policy failures, envelope errors |
| `internal/mailbox` | Mailbox aggregation, pagination, summaries, message filters, delivery waits |
| `internal/web` | Auth gates, bearer tokens, login/logout, password change, dashboard, HTMX fragments, legacy and v1 API, mailbox/wait/`.eml` endpoints, policies, events, stats, 404s, SSE, WebSocket, MCP tools, WebDAV read-only filesystem, OpenAPI serving |

## Storage test strategy

`internal/storage/store_test.go` defines one suite and executes it twice:

```go
for name, store := range stores(t) {
    t.Run(name, func(t *testing.T) {
        // shared behavior assertions
    })
}
```

Any new repository method must be exercised against both implementations. This
is the safety net for the Genji SQL subset.

## SMTP integration tests

Tests bind port `0` so the OS assigns a free port, then read the real address
from `Server.Addrs()`. They use `net/smtp.SendMail` to exercise the full
protocol path, and assert on stored messages and emitted events.

```go
srv, store, bus := startServer(t, testConfig(0))
addr := srv.Addrs()[0].String()
err := smtp.SendMail(addr, nil, "sender@example.org",
    []string{"inbox@example.test"}, []byte(sampleMessage))
```

## Web tests

Fiber's `app.Test` runs requests in-process. A small cookie helper performs a
real login and reuses the session cookie:

```go
resp := request(t, srv, http.MethodPost, "/login",
    strings.NewReader(form.Encode()), nil,
    map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
cookie := findSessionCookie(resp)
```

For WebSocket tests, start the app on a real listener and dial it with
`github.com/fasthttp/websocket`:

```go
ln, _ := net.Listen("tcp", "127.0.0.1:0")
go srv.App().Listener(ln)
conn, _, _ := websocket.DefaultDialer.Dial("ws://"+ln.Addr()+"/ws/events", headers)
```

## Coverage

The application packages cover roughly 89% of statements; the total including
the standalone integration runner is about 84%. Coverage is a guide, not a
target: the important cases here are envelope failures, policy precedence,
storage parity between memory and Genji, session/bearer boundaries, managed
credentials, Bleve search/fallback, WebDAV read-only enforcement, and MCP tool
results.

## Race detector

The full race suite is part of the expected verification for changes to storage,
events, SMTP, or web code:

```bash
make test-race
```

bcrypt cost 12 is slow under `-race`; the auth and web suites take noticeably
longer. Use generous HTTP client timeouts in race mode (30 seconds in the
WebSocket test) rather than lowering the production bcrypt cost.

## Integration runner

`make integration` builds the server and the standalone `mailvoid-it` client,
launches MailVoid on temporary free ports, and runs the JTD-validated suite in
`testdata/integration`. It exercises managed credentials, SMTP capture, REST,
MCP, WebDAV, `.eml` downloads, and Bleve search. `make integration-tui` provides
an interactive Charmbracelet selection and progress view.

See [Integration runner](/development/integration-runner/) for the JSON schema,
step types, assertions, and adding tests.

## Manual end-to-end check

```bash
make build
MAILVOID_ADMIN_PASSWORD=secret123 ./bin/mailvoid &

python3 - <<'PY'
import smtplib
from email.mime.text import MIMEText
m = MIMEText("hello")
m["Subject"] = "smoke test"
m["From"] = "a@b.test"
m["To"] = "x@y.test"
with smtplib.SMTP("127.0.0.1", 1025, timeout=5) as s:
    s.sendmail("a@b.test", ["x@y.test"], m.as_string())
PY

curl -s -c /tmp/cookies -d 'username=admin&password=secret123' \
  http://localhost:8025/login
curl -s -b /tmp/cookies http://localhost:8025/api/messages | jq
```

## What is not covered

- Browser rendering and JavaScript behavior (HTMX, Cash, theme, countdown) are
  exercised manually or by inspecting the served HTML.
- Load and soak testing are not part of the suite.
- External reverse proxy, TLS, and systemd integration are deployment concerns.
