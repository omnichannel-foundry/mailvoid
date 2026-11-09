---
title: Policies
weight: 30
---

# Policies

Policies decide what happens to a message on arrival. Each rule targets one
message field with a Go regular expression and chooses one action.

## Evaluation model

1. On `DATA`, MailVoid loads all **enabled** policies ordered by ascending
   `priority`, then by creation time for ties.
2. The first rule whose regex matches the target field wins.
3. If no rule matches, the message is kept permanently.

There is no "all matching rules" stacking. Order matters, and the console lets
you move rules up and down.

## Target fields

| Field | Value matched by the expression |
|-------|----------------------------------|
| `sender` | Envelope `MAIL FROM` address |
| `recipient` | Full `RCPT TO` address (`mailbox@domain`) |
| `mailbox` | Local part only |
| `domain` | Domain part only |
| `subject` | Decoded subject |
| `remote_ip` | TCP peer IP address |
| `local_port` | Receiving port as a decimal string |
| `any` | All of the above joined with spaces |

`any` is convenient for broad rules but can produce surprising matches because
the fields are concatenated. Prefer a specific field when possible.

## Actions

| Action | Effect | Message stored? | Expiry |
|--------|--------|-----------------|--------|
| `keep` | Retain permanently | Yes | None |
| `ttl` | Retain until a deadline | Yes | `now + ttl_seconds` |
| `drop` | Blackhole on arrival | No | Not applicable |

- `ttl` requires `ttl_seconds > 0`. For any other action, the stored
  `ttl_seconds` is normalized to `0`.
- `drop` returns success to the SMTP client, then discards the message. This is
  intentional blackhole behavior; use it when you want senders to believe the
  message was accepted.
- A TTL message keeps its matched policy ID and name so the detail page and API
  can explain why it exists.

## Regular expressions

MailVoid uses Go's `regexp` package, which implements RE2:

- linear-time matching, safe against catastrophic backtracking;
- **no backreferences** (`\1`) and **no lookaround** (`(?=...)`, `(?<=...)`);
- matching is case-sensitive by default;
- use `(?i)` for case-insensitive matching;
- anchors `^` and `$` apply to the whole field value.

Examples:

| Pattern | Matches |
|---------|---------|
| `@spam\.example$` | Senders ending in `@spam.example` |
| `(?i)^invoice` | Subjects starting with `invoice` in any case |
| `^billing@` | Recipients whose mailbox is `billing` |
| `^192\.0\.2\.` | Remote IPs in the documentation range |
| `^(1025|2525)$` | Either receiving port |
| `.*` | Everything |

Patterns longer than 2048 characters are rejected. Invalid patterns are
rejected at the API/form boundary and are skipped if they somehow reach storage.

## Creating rules

### Console

Use **Policies** → the create form. The fields are:

- **Name** — human-readable label; required.
- **Field** — one of the target fields.
- **Pattern** — the Go regular expression.
- **Action** — `keep`, `ttl`, or `drop`.
- **TTL seconds** — required for `ttl`.
- **Enabled** — checked by default.

The rule is appended to the end of the evaluation order.

### API

```bash
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{
        "name": "expire notifications after one hour",
        "field": "subject",
        "pattern": "(?i)^notification",
        "action": "ttl",
        "ttl_seconds": 3600,
        "enabled": true
      }' \
  http://localhost:8025/api/policies
```

## Editing and ordering

- **Edit** opens the rule in the form panel. Saving preserves its position,
  creation time, and ID.
- **↑ / ↓** swap the rule with its neighbor and renumber priorities from zero.
- **Enable / Disable** toggles evaluation without deleting the rule.
- **Delete** removes it immediately.

The API equivalents:

```bash
# full update
curl -s -b cookies.txt -X PUT \
  -H 'Content-Type: application/json' \
  -d '{
        "name": "renamed",
        "field": "sender",
        "pattern": "@spam\\.test$",
        "action": "drop",
        "enabled": true
      }' \
  http://localhost:8025/api/policies/<policy-id>

# reorder: every policy id exactly once
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"ids":["<second-id>","<first-id>","<third-id>"]}' \
  http://localhost:8025/api/policies/reorder
```

`PUT` is a full replacement for the rule's text fields: send every string and
`ttl_seconds` you want to keep. Omitted strings and `ttl_seconds` become zero
values, which usually fails validation. Omitted `enabled` and `priority` preserve
the existing values.

## Testing expressions

The **Regex tester** panel evaluates a field, pattern, and sample value without
saving a rule:

```bash
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"field":"subject","pattern":"^invoice","value":"Invoice 42"}' \
  http://localhost:8025/api/policies/test
```

```json
{"field":"subject","matched":true,"pattern":"^invoice","value":"Invoice 42"}
```

The tester is a match check only. It does not show which action would win
against the full ordered policy list.

## TTL deletion

TTL is enforced by a sweeper goroutine:

- runs once at startup, then every `MAILVOID_SWEEP_INTERVAL`;
- calls `DeleteExpired(now)` on the repository;
- removes messages whose `expires_at` is non-zero and `<= now`;
- emits `message.deleted` per message with `reason: "ttl expiry"`.

A message can remain visible for up to one sweep interval after its timer
expires.

## Recipe ideas

| Goal | Field | Pattern | Action |
|------|-------|---------|--------|
| Discard known spam sender | `sender` | `@spam\.test$` | `drop` |
| Keep invoices forever | `subject` | `(?i)invoice` | `keep` |
| Expire all notifications in a day | `subject` | `(?i)^notification` | `ttl` 86400 |
| Expire everything on the test port | `local_port` | `^3025$` | `ttl` 3600 |
| Drop mail from a lab subnet | `remote_ip` | `^10\.20\.` | `drop` |
| Expire a specific mailbox | `mailbox` | `^qa-automation$` | `ttl` 7200 |

Place specific `drop` rules before broad `keep` rules. Because first match wins, a
catch-all `keep` rule at the top makes every later rule unreachable.
