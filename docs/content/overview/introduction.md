---
title: Introduction
weight: 10
---

# Introduction

MailVoid is a wildcard SMTP sink and mail inspector. It listens for SMTP on one
or more ports, accepts mail for any mailbox on the domains you allow, and stores
the envelope metadata, headers, decoded text and HTML bodies, and the raw
message.

Unlike a full mail server, MailVoid does not deliver mail anywhere. There is no
outbound queue, no relay, no IMAP or POP3, and no mailbox protocol. It is a
capture device for development, testing, automation, and controlled inspection
of inbound mail.

## Why it exists

Testing mail flows is awkward. Real providers require credentials and network
access, local mail servers are heavy, and temporary inbox services are outside
your control. MailVoid fills the gap:

- one binary with an embedded database;
- any number of SMTP ports and domains;
- a web dashboard with live events and message inspection;
- regex rules that decide what to keep, expire, or discard;
- a JSON API for automation;
- no external runtime dependencies.

## Design principles

### Event-first

Every meaningful state transition is an event. A connection opening, `MAIL FROM`
being parsed, `RCPT TO` being accepted or rejected, a policy matching, a message
being stored, a TTL deletion, and a login all emit structured JSON. Events are
persisted, indexed, and broadcast live.

This makes the audit trail a first-class product surface rather than a log file
someone might read later.

### Repository seam

All persistence goes through a small `storage.Store` interface. The production
implementation uses Genji; tests use the in-memory implementation. Swapping the
database does not touch the SMTP, policy, event, or web layers.

### Capture everything, decide later

The raw message and the envelope are always preserved when a message is kept.
The dashboard can show a sandboxed HTML preview while the API can return the
original bytes. Policy decisions are recorded on the message so an operator can
see why it is present and when it will expire.

### Single binary

Templates, CSS, JavaScript, and the database engine are compiled into the
executable. Deployment is copying one file and choosing a data directory.

## Who it is for

- Developers testing transactional email, notification flows, or inbound mail
  parsing.
- QA and integration environments that need deterministic capture.
- Operators who need a controlled SMTP sink with retention rules and an audit
  trail.
- Security researchers inspecting message headers and MIME structure in a
  sandboxed viewer.

## Non-goals

MailVoid is not:

- an MTA or relay;
- an IMAP/POP3 server;
- a production mailbox provider;
- an SMTP-authenticated or STARTTLS service;
- a multi-tenant system with role separation (every authenticated account is an
  administrator).

> [!WARNING]
> With `MAILVOID_DOMAINS=*` (the default), every domain is accepted. Never expose
> an unrestricted MailVoid listener to the public internet: it can be used to
> fill your disk or to launder traffic. Restrict domains and firewall the SMTP
> ports, or place the service on a private network.
