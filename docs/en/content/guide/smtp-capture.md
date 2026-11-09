---
title: SMTP capture
weight: 10
---

# SMTP capture

## Listeners

MailVoid opens one TCP listener for every configured SMTP port. All listeners
share the same backend, database, policy engine, and event bus. A message
received on port `2525` is identical to one received on `1025` except for its
recorded `local_port`.

```text
MAILVOID_SMTP_HOST=0.0.0.0
MAILVOID_SMTP_PORTS=1025,2525,3025
```

The server does not use TLS and does not advertise `STARTTLS` or `AUTH`. It is
plain SMTP on the configured ports.

> [!WARNING]
> SMTP credentials sent to MailVoid are transmitted in the clear. Only accept mail
> from trusted networks, or tunnel SMTP through a private network such as a VPN
> or WireGuard.

## Envelope capture

For each accepted recipient, MailVoid records:

| Field | Source |
|-------|--------|
| `remote_ip` | TCP peer address |
| `local_port` | Port of the listener that accepted the message |
| `sender` | `MAIL FROM` address (empty for `<>` bounces) |
| `recipient` | Full `RCPT TO` address, lower-cased |
| `mailbox` | Local part before `@` |
| `domain` | Domain part after `@`, lower-cased |
| `received_at` | Server time in UTC |
| `size` | Raw message size in bytes |
| `subject` | Decoded `Subject` header (RFC 2047) |
| `headers` | Parsed header map |
| `raw_headers` | Header block exactly as received |
| `raw` | Complete raw message |
| `body_text` | Decoded `text/plain` body |
| `body_html` | Decoded `text/html` body |

Envelope addresses are lower-cased. Display names in envelope commands are
normalized to the bare address when `net/mail` can parse them.

## Domain acceptance

`RCPT TO` is evaluated immediately:

- If the address is malformed: `501 invalid recipient address`.
- If the domain is not allowed: `550 relay access denied`, with an
  `smtp.rcpt_to` event containing `accepted:false` and `reason`.
- If the domain is allowed: the recipient is queued for `DATA`.

The `MAIL FROM` sender is not restricted by the domain list.

## Multiple recipients

A message sent to multiple recipients in one transaction is stored once per
recipient. Each copy carries the same raw bytes, sender, subject, and bodies, but
its own `recipient`, `mailbox`, and `domain`. Policies are evaluated separately
for each recipient, so a rule targeting `mailbox=billing` does not affect a
message delivered to `mailbox=alerts`.

## MIME decoding

The parser handles:

- `multipart/*` recursively;
- `base64` and `quoted-printable` transfer encodings;
- RFC 2047 encoded words in the subject;
- UTF-8, US-ASCII, ISO-8859-1/Latin-1, and Windows-1252 charsets;
- text and HTML alternatives.

Attachments are not included in `body_text` or `body_html`, but remain available
in the raw message. `message/rfc822` parts are not recursively parsed. See
[MIME parsing](/reference/mime-parsing/) for the exact rules.

## SMTP responses

| Condition | Response |
|-----------|----------|
| Recipient accepted, message stored | `250 OK` |
| Recipient domain not accepted | `550 relay access denied` |
| Malformed recipient | `501 invalid recipient address` |
| Malformed sender | `501 invalid sender address` |
| No recipients before `DATA` | `503 no recipients` |
| Message larger than the limit | `552 message too large` |
| Storage failure | `451 temporary storage failure` |
| Policy evaluation error | `250 OK` with the message kept and a warning event |

`451` tells a well-behaved client to retry. `550`, `501`, `503`, and `552` are
permanent failures for that transaction.

## Unparsable messages

`net/mail` can reject malformed header blocks. MailVoid does not lose the
message:

- the raw bytes are stored;
- the subject becomes `(unparsable message)`;
- the decoded bodies are empty;
- an `smtp.error` event with `stored:true` and the parser error is emitted.

The original raw message remains available in the detail view and API.

## Policy interaction

Policies are applied after the full message is read and parsed.

- `keep` or no matching rule: the message is stored and a `message.received`
  event is emitted.
- `ttl`: the message is stored with `action=ttl`, `expires_at`, and policy
  metadata.
- `drop`: the message is not stored; a `message.dropped` event is emitted. The
  SMTP client still receives `250 OK`.

## Example session

```text
$ swaks --server 127.0.0.1:1025 \
    --from sender@example.org \
    --to inbox@example.test \
    --header "Subject: capture test" \
    --body "hello"

<-  220 mailvoid.local ESMTP
 ->  EHLO localhost
<-  250-mailvoid.local
 ->  MAIL FROM:<sender@example.org>
<-  250 OK
 ->  RCPT TO:<inbox@example.test>
<-  250 OK
 ->  DATA
<-  354 Start mail input
...
<-  250 OK
```

In the audit trail this produces `smtp.connected`, `smtp.mail_from`,
`smtp.rcpt_to`, `smtp.data`, and `message.received` (plus `smtp.closed` when the
session ends).
