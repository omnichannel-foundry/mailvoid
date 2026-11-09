---
title: Security
weight: 50
---

# Security

MailVoid is an administrative tool that stores complete inbound messages,
including headers and bodies. Treat the data directory and the dashboard as
sensitive.

## Threat model

| Asset | Risk |
|-------|------|
| Stored messages | Contain PII, credentials in reset links, attachments, internal headers |
| Raw messages | May include malware or tracking pixels |
| Event log | Contains subjects, senders, recipients, IPs, usernames |
| Admin credentials | Full access to all mail, policies, and settings |
| SMTP listener | Unauthenticated; anyone who can reach it can submit mail |
| Database directory | Unencrypted at rest |

## Network exposure

- **Restrict SMTP.** MailVoid does not authenticate SMTP clients. A public
  listener with `MAILVOID_DOMAINS=*` is an open sink: anyone can fill the disk.
- **Bind HTTP to loopback** and terminate TLS at a reverse proxy.
- **Firewall management ports.** Do not expose the dashboard or API to the
  public internet without authentication and TLS.
- **Prefer a private network** (VPN, WireGuard, private VPC) for SMTP between
  application hosts and MailVoid.

## SMTP has no TLS or authentication

There is no `STARTTLS`, no implicit TLS listener, and no `AUTH`. Credentials and
message content travel in cleartext on the SMTP hop. This is a deliberate
minimalist constraint, not a configuration option.

If SMTP confidentiality is required, tunnel the hop through a private network or
place an SMTP-aware gateway in front of MailVoid.

## Dashboard authentication

- Passwords are bcrypt hashes at cost 12.
- The session cookie is HTTP-only and `SameSite=Lax`; 12-hour expiry.
- Set `MAILVOID_COOKIE_SECURE=true` behind HTTPS.
- Every authenticated account is an administrator. There is no least-privilege
  role model.
- Login rate limiting and CSRF tokens are not implemented. Use network
  restrictions or a proxy that can add rate limiting and CSRF protection.
- The generated first-run password is printed to the process log. Protect
  journal access and rotate the password after signing in.

## Secrets

Do not commit `MAILVOID_ADMIN_PASSWORD`, `MAILVOID_API_TOKEN`, or
`MAILVOID_WEBDAV_PASSWORD`. Use a secret manager or systemd `EnvironmentFile`
with mode `0600`. Flags that carry secrets are visible in the process list;
prefer environment variables or an environment file.

- `MAILVOID_API_TOKEN` grants read and delete access to every message through
  `/api/v1` and `/mcp`. Rotate it by changing the value and restarting.
- `MAILVOID_WEBDAV_PASSWORD` grants read access to every mailbox through
  `/dav/`. It is separate from the administrator account and API token.
- WebDAV uses HTTP Basic authentication. Without TLS, the password is visible on
  the network; always terminate HTTPS in front of it.

## Data at rest

The Genji database and JSONL event log are not encrypted. Use full-disk or
volume encryption, restrictive file permissions (`0750` directory), and backups
that are encrypted in transit and at rest.

File permissions:

```bash
chown -R mailvoid:mailvoid /var/lib/mailvoid
chmod 0750 /var/lib/mailvoid
chmod 0600 /var/lib/mailvoid/events.jsonl
```

## Email content safety

- HTML bodies are rendered in a sandboxed iframe without scripts or same-origin
  access.
- Plain text and raw messages are HTML-escaped before display.
- Remote images and styles inside the sandboxed HTML can still make network
  requests. If that is a concern, inspect the raw message instead.
- Attachments are stored inside the raw message. MailVoid does not scan for
  malware. Do not open extracted attachments on a trusted host.
- Email headers can contain forged senders; `sender` is the SMTP envelope, not a
  cryptographic identity.

## Audit log sensitivity

Events include subjects, sender and recipient addresses, source IPs, usernames,
and policy patterns. They may contain personal data. Apply the same access
controls and retention policy to `events.jsonl` and the Genji `events` table as
to the messages themselves.

Event history is unbounded. Define an external retention/archival process if
regulatory or disk constraints require it.

## Search index sensitivity

The Bleve index contains analyzed message terms and stored searchable fields.
Treat `search.bleve/` as sensitive as the Genji database. Access controls,
backups, and disk encryption must cover it. It is rebuildable from Genji, so it
can be omitted from backups when rebuild time is acceptable.

## Managed credential risks

- API keys and WebDAV users are managed over `/api/v1/admin`, which requires an
  administrator session or the bootstrap API token. Managed API keys cannot
  manage credentials.
- Managed API keys can read and delete all captured mail. Treat them as
  administrator-adjacent secrets and rotate them by creating a replacement and
  deleting the old key.
- Managed WebDAV users can read every mailbox and are not scoped per mailbox.
- Generated tokens/passwords are returned once. Store them in a secret manager.

## Agent and file-surface risks

- MCP includes `delete_message`. An agent with the API token can permanently
  destroy captured mail.
- `/api/v1`, `/mcp`, and the console all expose raw message content; treat the
  API token as an administrator credential.
- WebDAV exposes the same raw content to anyone with the Basic-auth pair and has
  no per-mailbox authorization.
- `.eml` downloads are the exact stored bytes, including attachments and active
  content. Do not open them on a trusted host without scanning.
- WebDAV and MCP are stateless; there are no per-session revocations. Rotate
  credentials and restart to invalidate old ones.

## Denial of service

- `MAILVOID_MAX_MESSAGE_BYTES` caps a single message (25 MiB default).
- There is no per-client rate limit or connection limit.
- Every message is stored once per recipient.
- Event persistence adds write amplification for each SMTP transaction.
- The event table and JSONL file grow without compaction.

For untrusted senders, restrict domains, firewall the SMTP ports, monitor disk,
and set a message-size limit appropriate to the workload.

## Dependency and runtime risks

- Genji v0.15.3 is pinned because newer releases renamed the module to Chai and
  require a newer Go toolchain. Track upstream security advisories and plan a
  repository replacement or migration if needed.
- Embedded frontend assets (DaisyUI, HTMX, Cash) are pinned in the repository.
  Review upgrades deliberately.
- The binary is statically linked; keep the build toolchain patched.

## Hardening checklist

- [ ] SMTP ports reachable only from trusted networks
- [ ] `MAILVOID_DOMAINS` restricted to the domains you actually use
- [ ] HTTP bound to loopback
- [ ] TLS reverse proxy in front of the dashboard
- [ ] `MAILVOID_COOKIE_SECURE=true`
- [ ] Long, unique administrator password from a secret manager
- [ ] Data directory permissions `0750`, files `0600`
- [ ] Disk encryption for the data volume
- [ ] Log and event access restricted
- [ ] Backup process tested
- [ ] Disk-space and service-health monitoring
- [ ] Message-size limit reviewed
- [ ] External event-retention/archival process defined
- [ ] `MAILVOID_API_TOKEN` set, stored in a secret manager, and rotated on a schedule
- [ ] WebDAV disabled, or `MAILVOID_WEBDAV_PASSWORD` set and served only over HTTPS
- [ ] MCP clients trusted with the API token and destructive `delete_message` access
- [ ] Search index directory covered by the same access controls and encryption as the database
