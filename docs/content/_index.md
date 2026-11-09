---
title: Documentation
bookToC: true
---

# MailVoid

MailVoid is a single-binary, event-first SMTP capture server with a built-in
administrative console. It accepts mail for any mailbox across multiple ports
and domains, stores the envelope and MIME content in an embedded Genji database,
applies ordered regular-expression retention rules, and streams a structured
JSON audit trail to the browser in real time.

{{% columns %}}
- **Wildcard capture**  
  Listen on several SMTP ports at once and accept any local mailbox for every
  configured domain.

- **Event-first core**  
  Connections, envelope commands, policy matches, storage, deletions, and logins
  all become structured events persisted to Genji and an append-only JSONL file.

- **Regex retention**  
  Keep, expire after a TTL, or blackhole mail on arrival using ordered Go RE2
  rules. Reorder and test rules from the dashboard or the JSON API.

- **One binary**  
  Fiber, Go templates, HTMX, DaisyUI/Tailwind, Cash, the Genji database, and all
  assets are compiled into a single static executable.
{{% /columns %}}

{{< button href="/getting-started/quick-start/" >}}Quick start{{< /button >}}
{{< button href="/overview/architecture/" >}}Architecture{{< /button >}}
{{< button href="/reference/api/" >}}API reference{{< /button >}}

## At a glance

| Property | Value |
|----------|-------|
| Language | Go 1.24.2+ |
| Web framework | Fiber v2 |
| Database | Genji v0.15.3 (embedded, Pebble-backed) |
| Default SMTP ports | `1025`, `2525` |
| Default dashboard | `http://localhost:8025` |
| Default data directory | `./data` |
| Authentication | bcrypt credentials + cookie sessions |
| Default admin | `admin` with a generated password when none is configured |
| Event transports | JSONL file, Genji table, Server-Sent Events, WebSocket |
| Agent API | `/api/v1` with OpenAPI 3.1 and bearer/session auth |
| Agent tooling | MCP Streamable HTTP at `/mcp` |
| File access | `.eml` downloads and read-only WebDAV at `/dav/` |
| Search | Embedded Bleve full-text index at `/api/v1/search` |
| Credential management | Administrator API for API keys and WebDAV users |
| Integration runner | `mailvoid-it` TUI/batch client with JTD test files |

## Where to go next

- New to MailVoid? Start with [Installation](/getting-started/installation/) and
  the [Quick start](/getting-started/quick-start/).
- Operating it? Read [Configuration](/getting-started/configuration/),
  [Deployment](/operations/deployment/), and [Security](/operations/security/).
- Building against it? Read the [API reference](/reference/api/),
  [OpenAPI](/reference/openapi/), [MCP server](/reference/mcp/), and the
  [Event reference](/reference/events/).
- Connecting agents or file clients? Read [MCP](/reference/mcp/) and
  [WebDAV](/operations/webdav/).
- Working on the code? Read [Project layout](/development/project-layout/) and
  [Testing](/development/testing/).

> [!NOTE]
> MailVoid is a capture appliance, not a mail transfer agent. It never relays
> outbound mail, and it does not implement SMTP authentication or STARTTLS.
> Bind it to a trusted network or put it behind a firewall.
