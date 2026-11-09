---
title: WebDAV
weight: 45
---

# WebDAV

MailVoid exposes a **read-only WebDAV tree** at `/dav/` for listing mailboxes
and downloading messages as `.eml` files.

## Enable

WebDAV can be enabled in two ways:

1. A bootstrap password (`MAILVOID_WEBDAV_PASSWORD`) configured at startup.
2. Creating a managed WebDAV user through `/api/v1/admin/webdav-users`.

Managed users are created, listed, and revoked through the administrator API;
see [Managed credentials](/reference/credentials/). Successful Basic-auth checks
are cached for five minutes.

Bootstrap credentials:

```bash
MAILVOID_WEBDAV_USER=mailvoid \
MAILVOID_WEBDAV_PASSWORD='replace-with-a-strong-password' \
./mailvoid
```

| Variable | Default | Meaning |
|----------|---------|---------|
| `MAILVOID_WEBDAV_USER` | `mailvoid` | Basic-auth username |
| `MAILVOID_WEBDAV_PASSWORD` | *(empty)* | Basic-auth password; empty disables WebDAV |

Equivalent flags: `-webdav-user`, `-webdav-password`.

When neither a bootstrap password nor any managed user exists, `/dav/` returns
`404`. When enabled, unauthenticated requests receive `401` with a
`WWW-Authenticate: Basic` challenge.

> [!WARNING]
> Basic authentication over plain HTTP exposes the password on the network.
> Serve MailVoid through an HTTPS reverse proxy (and set
> `MAILVOID_COOKIE_SECURE=true` for the console). WebDAV does not use the
> administrator session or API token; it has its own credentials.

## URL layout

```text
/dav/                                  mailbox directories
/dav/inbox@example.test/               messages in that mailbox
/dav/inbox@example.test/msg_abc.eml    one raw RFC 5322 message
```

- Directory names are `mailbox@domain`.
- File names are `<message-id>.eml`.
- Only mailboxes with at least one captured message are listed.
- Files are the exact bytes stored by MailVoid, with `Content-Type:
  message/rfc822`.

## Allowed methods

| Method | Behavior |
|--------|----------|
| `OPTIONS` | Capabilities |
| `PROPFIND` | List directories and files |
| `GET`, `HEAD` | Download a message or inspect metadata |

Everything else returns `405 Method Not Allowed` with an `Allow` header. This is
not a backup target: `PUT`, `DELETE`, `MKCOL`, `MOVE`, `COPY`, `LOCK`,
`PROPPATCH`, and `UNLOCK` are rejected.

## Examples

### List mailboxes

```bash
curl -s -u 'mailvoid:replace-with-a-strong-password' \
  -X PROPFIND -H 'Depth: 1' \
  --data '<?xml version="1.0"?><propfind xmlns="DAV:"><allprop/></propfind>' \
  http://localhost:8025/dav/
```

The response is `207 Multi-Status` XML containing mailbox directories.

### List messages in a mailbox

```bash
curl -s -u 'mailvoid:replace-with-a-strong-password' \
  -X PROPFIND -H 'Depth: 1' \
  --data '<?xml version="1.0"?><propfind xmlns="DAV:"><allprop/></propfind>' \
  'http://localhost:8025/dav/inbox@example.test/'
```

### Download one message

```bash
curl -u 'mailvoid:replace-with-a-strong-password' \
  -o message.eml \
  'http://localhost:8025/dav/inbox@example.test/msg_81997d51ccdda1a5.eml'
```

### Mount with davfs2

Read-only mounts work with a standard WebDAV client:

```bash
sudo mount -t davfs -o ro \
  http://localhost:8025/dav/ /mnt/mailvoid
```

Provide the WebDAV username and password when prompted. Because the tree is
read-only, writes fail at both the client and the server.

## Relationship to the console and API

| Surface | Use |
|---------|-----|
| Console | Human inspection; **Download .eml** button on a message |
| `/api/v1/messages/{id}/eml` | Scripted `.eml` download with bearer or session auth |
| `/dav/` | WebDAV clients and file-manager mounts |

All three return the same stored raw message.

## Limits

- WebDAV authentication is a separate global credential, not per-user.
- Directory aggregation and message listing scan the embedded store; very large
  installations make `PROPFIND` slower.
- There is no write, move, lock, or property mutation support.
- There is no per-message authorization; anyone with the WebDAV credentials can
  read every mailbox.
- Use HTTPS. Basic auth is not encrypted by itself.
