---
title: Deployment
weight: 10
---

# Deployment

## Binary and data directory

Deploy the release binary and choose a writable state directory:

```bash
sudo install -D -m 0755 bin/mailvoid /usr/local/bin/mailvoid
sudo install -d -o mailvoid -g mailvoid /var/lib/mailvoid
```

Example environment file (`/etc/mailvoid/mailvoid.env`, mode `0600`):

```bash
MAILVOID_SMTP_HOST=0.0.0.0
MAILVOID_SMTP_PORTS=1025,2525
MAILVOID_HTTP_ADDR=127.0.0.1:8025
MAILVOID_DOMAINS=app.test,*.mail.test
MAILVOID_DATA_DIR=/var/lib/mailvoid
MAILVOID_ADMIN_USER=admin
MAILVOID_ADMIN_PASSWORD=change-me-from-a-secret-manager
MAILVOID_COOKIE_SECURE=true
# Optional agent and WebDAV surfaces:
# MAILVOID_API_TOKEN=replace-with-a-long-random-token
# MAILVOID_WEBDAV_USER=mailvoid
# MAILVOID_WEBDAV_PASSWORD=replace-with-another-secret
```

Start under a process supervisor. The [systemd unit](/operations/systemd/) is a
complete example.

## Network layout

Typical deployment:

```text
SMTP clients ── 1025/2525 ──▶ MailVoid ── 127.0.0.1:8025 ──▶ TLS reverse proxy ──▶ browser
```

- Bind HTTP to loopback and let a reverse proxy terminate TLS.
- Expose only the SMTP ports you need.
- Restrict SMTP with firewall rules or a private network. MailVoid does not
  authenticate SMTP clients.
- Set `MAILVOID_COOKIE_SECURE=true` when HTTPS is used end to end.

## Ports below 1024

The defaults (`1025`, `2525`) are unprivileged. To bind port `25` or `587`, grant
the capability instead of running as root:

```ini
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
```

## Clock

Event and message timestamps come from the host clock. Run NTP or
`systemd-timesyncd`. All persisted times are UTC; the console renders UTC while
the live event client shows local time.

## Disk

Plan for:

- raw messages, decoded bodies, and headers in the Genji database (one copy per
  envelope recipient);
- the append-only event table and JSONL log;
- the database's write-ahead log and compaction headroom.

There is no built-in quota or retention for events. Monitor free space and set
an external alert.

## Single instance

Genji holds a `LOCK` on its directory. Do not run two MailVoid processes against
the same data directory. To scale horizontally, give each instance its own data
directory, ports, and HTTP address, and route clients explicitly.

## Reverse proxy and TLS

The dashboard and API should be served over HTTPS. See
[Reverse proxy](/operations/reverse-proxy/) for Nginx and Caddy examples,
including WebSocket and SSE settings.

> [!WARNING]
> HTTPS on the dashboard does not encrypt SMTP traffic. MailVoid has no STARTTLS.
> Send mail over a trusted network or a tunnel.

## Container note

There is no official container image in this repository. A container built from
the release binary works if you:

- mount `/var/lib/mailvoid` for persistence;
- publish SMTP and HTTP ports;
- set the environment variables above;
- keep the data directory on a single instance.

Because the image is not shipped, follow your organization's base-image and
supply-chain policy.

## Health checks

Use `GET /healthz` for liveness. It is public, cheap, and does not touch the
database:

```bash
curl -fsS http://127.0.0.1:8025/healthz
```

A deeper readiness check is `GET /api/stats` with a valid session; it queries
messages, policies, and events.
