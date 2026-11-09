---
title: Quick start
weight: 20
---

# Quick start

This walkthrough captures a message, signs in, and exercises a policy.

## 1. Start MailVoid

```bash
MAILVOID_ADMIN_USER=admin \
MAILVOID_ADMIN_PASSWORD=secret123 \
MAILVOID_SMTP_PORTS=1025,2525 \
MAILVOID_HTTP_ADDR=:8025 \
./bin/mailvoid
```

The default configuration already uses the same ports and HTTP address; the
explicit values make the example self-documenting.

## 2. Send a message

Using `swaks`:

```bash
swaks --server 127.0.0.1:1025 \
  --from sender@example.org \
  --to inbox@example.test \
  --header "Subject: Hello from MailVoid" \
  --body "This is the plain-text body."
```

Using Python's standard library:

```python
import smtplib
from email.mime.text import MIMEText

msg = MIMEText("This is the plain-text body.")
msg["Subject"] = "Hello from MailVoid"
msg["From"] = "sender@example.org"
msg["To"] = "inbox@example.test"

with smtplib.SMTP("127.0.0.1", 1025, timeout=5) as smtp:
    smtp.sendmail("sender@example.org", ["inbox@example.test"], msg.as_string())
```

Send a multipart message to the second port to verify multi-port capture:

```python
from email.mime.multipart import MIMEMultipart
from email.mime.text import MIMEText
import smtplib

msg = MIMEMultipart("alternative")
msg["Subject"] = "Multipart on 2525"
msg["From"] = "sender@example.org"
msg["To"] = "inbox@example.test"
msg.attach(MIMEText("plain text"))
msg.attach(MIMEText("<p>HTML body</p>", "html"))

with smtplib.SMTP("127.0.0.1", 2525, timeout=5) as smtp:
    smtp.sendmail("sender@example.org", ["inbox@example.test"], msg.as_string())
```

## 3. Sign in

Open <http://localhost:8025> and sign in with `admin` / `secret123`.

You can also authenticate with curl and keep the session cookie:

```bash
curl -s -c cookies.txt \
  -d 'username=admin&password=secret123' \
  http://localhost:8025/login
```

## 4. Inspect the capture

- **Dashboard** shows totals and the most recent messages and events.
- **Messages** lists every captured message with the receiving port, source IP,
  target mailbox, sender, subject, and action.
- Open a message to see envelope metadata, the sandboxed HTML preview, the text
  body, headers, and raw bytes.

Programmatically:

```bash
curl -s -b cookies.txt http://localhost:8025/api/messages | jq
curl -s -b cookies.txt http://localhost:8025/api/messages/<id> | jq
```

## 5. Add a retention policy

Create a rule that drops mail from a known spam domain:

```bash
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{
        "name": "drop spam",
        "field": "sender",
        "pattern": "@spam\\.test$",
        "action": "drop",
        "enabled": true
      }' \
  http://localhost:8025/api/policies | jq
```

Send a message from `promo@spam.test` to `inbox@example.test`. The SMTP client
sees success, but the message is discarded. Confirm with the message list and the
event stream:

```bash
curl -s -b cookies.txt \
  'http://localhost:8025/api/events?type=message.dropped' | jq
```

Create a TTL rule:

```bash
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{
        "name": "expire after 60 seconds",
        "field": "subject",
        "pattern": "temporary",
        "action": "ttl",
        "ttl_seconds": 60,
        "enabled": true
      }' \
  http://localhost:8025/api/policies | jq
```

Send a message with `Subject: temporary test`. It is stored with
`"action":"ttl"` and an `expires_at` value; the sweeper removes it after the TTL
and emits `message.deleted` with reason `ttl expiry`.

## 6. Watch events live

Open **Event stream** in the console. The page loads recent persisted events and
then prepends new ones over WebSocket. The status dot is green while the socket
is connected. Use **Pause** to freeze the view and **Clear view** to remove
client-side rows without touching the audit log.

Programmatic alternatives:

```bash
# Server-Sent Events
curl -N -b cookies.txt http://localhost:8025/api/events/stream

# JSON history
curl -s -b cookies.txt http://localhost:8025/api/events | jq
```

## 7. Stop

Press `Ctrl-C` or send `SIGTERM`. MailVoid closes the SMTP listeners, the HTTP
server, live streams, the event log, and the database in that order.
