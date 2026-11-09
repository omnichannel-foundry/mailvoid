---
title: Dashboard tour
weight: 60
---

# Dashboard tour

The console is a server-rendered Fiber application. HTMX updates the message
list and policy test panel without a full page reload; Cash handles theme
switching, clipboard copy, and the mobile drawer.

## Navigation

| Page | Purpose |
|------|---------|
| **Dashboard** | Counts, recent messages, and recent events |
| **Messages** | Search and filter every stored message |
| **Event stream** | Live audit trail with type/severity filters |
| **Policies** | Create, edit, reorder, enable, test, and delete rules |
| **Settings** | Runtime configuration and password change |

The sidebar shows the version and a live-event indicator. On narrow screens the
top-left button opens the drawer.

## Dashboard

Four cards summarize:

- total stored messages;
- messages received in the last 24 hours;
- number of policies;
- total number of events.

Below them are the eight most recent messages and the eight most recent events,
each linking to the full page.

## Messages

The list supports live filtering:

- typing in **Search** triggers a debounced HTMX request (350 ms);
- changing a select updates immediately;
- the URL is updated so the filtered view is linkable;
- pagination shows total matches and page count.

Rows link to the detail view. Empty results explain that no message matched and
prompt you to send mail.

## Message detail

Sections, in order:

1. **Header** — subject, sender, recipient, action badge, policy badge, and a
   live expiry badge for TTL messages.
2. **Envelope** — mailbox, source IP, receiving port, receive time, size,
   message ID, policy, and expiry.
3. **Origin** — a concise summary of which listener and peer produced the
   message.
4. **HTML body** — sandboxed iframe preview when an HTML part exists.
5. **Plain text body** — copyable decoded text.
6. **Headers** — parsed table and raw header block, copyable.
7. **Raw message** — the exact bytes, copyable.

The **Download .eml** button returns the raw RFC 5322 message as an attachment.
The **Delete** button asks for confirmation and permanently removes the message.

## Event stream

The page loads recent events from the database and then subscribes to
`/ws/events`. Controls:

- **live dot** — green while connected, amber while reconnecting;
- **Pause / Resume** — buffer incoming events in the browser;
- **Clear view** — remove client-side rows without deleting history.

Each row shows time, severity badge, type, and expandable JSON payload.

## Policies

The page has three panels:

1. **Rule form** — create a rule, or edit an existing one in place. The form is
   replaced by an edit form when you press **Edit**; **Cancel** reloads the
   create form.
2. **Regex tester** — POSTs a field, pattern, and sample value via HTMX and shows
   match/no-match or the regex error.
3. **Rule table** — ordered by priority with edit, move up/down, enable/disable,
   and delete actions.

The evaluation-order card restates the first-match-wins rule so operators do not
put a catch-all rule above specific drop rules by accident.

## Settings

Settings displays:

- a live mail-capture on/off toggle;
- SMTP host, ports, accepted domains, and maximum message size;
- HTTP address;
- Genji database path and JSONL event log path;
- sweeper interval, secure-cookie setting, API-token configuration state,
  WebDAV enabled state, and the search index path;
- total stored messages;
- the administrator password change form.

Configuration is read at startup. Changing environment variables requires a
restart; the capture toggle applies immediately and is persisted in the
database. The settings page is otherwise informational except for the password
form.

## Theme

The initial theme is dark. The moon/sun button toggles between dark and light
and stores the choice in `localStorage` under `mailvoid-theme`. An inline head
script applies the saved theme before paint to avoid a flash.
