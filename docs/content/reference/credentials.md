---
title: Managed credentials
weight: 22
---

# Managed credentials

MailVoid can create and revoke **API keys** and **WebDAV users** at runtime.
They are stored in the embedded database and take effect immediately; no restart
is required.

The bootstrap credentials from the environment remain supported:

- `MAILVOID_API_TOKEN` — an administrator bearer token.
- `MAILVOID_WEBDAV_USER` / `MAILVOID_WEBDAV_PASSWORD` — the original WebDAV
  Basic-auth pair.

## Administrator-only management

Credential management routes are under `/api/v1/admin`. They require either:

- an authenticated administrator **session** (`POST /login`); or
- the bootstrap `MAILVOID_API_TOKEN` bearer token.

Managed API keys are deliberately **not** allowed to create, list, or delete
credentials. This prevents privilege escalation from an agent key.

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/admin/api-keys` | List managed keys |
| `POST` | `/api/v1/admin/api-keys` | Create a key |
| `DELETE` | `/api/v1/admin/api-keys/{id}` | Revoke a key |
| `GET` | `/api/v1/admin/webdav-users` | List managed WebDAV users |
| `POST` | `/api/v1/admin/webdav-users` | Create a WebDAV user |
| `DELETE` | `/api/v1/admin/webdav-users/{username}` | Delete a WebDAV user |

## API keys

Create a key with the administrator session:

```bash
curl -s -c cookies.txt \
  -d 'username=admin&password=change-me' \
  http://localhost:8025/login

curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"name":"ci-agent","expires_at":"2027-01-01T00:00:00Z"}' \
  http://localhost:8025/api/v1/admin/api-keys
```

The response contains the **plaintext token exactly once**:

```json
{
  "id": "key_...",
  "name": "ci-agent",
  "prefix": "a1b2c3d4e5f6",
  "created_at": "2026-11-08T05:00:00Z",
  "expires_at": "2027-01-01T00:00:00Z",
  "token": "mv_a1b2c3d4e5f6_..."
}
```

Use it as `Authorization: Bearer mv_...` on `/api/v1` and `/mcp`. Only a
SHA-256 hash of the token is stored; the list endpoint returns the key ID, name,
prefix, creation time, expiry, and creator, never the token or hash.

- Names are required.
- `expires_at` is optional and must be in the future.
- A managed key has the same agent capabilities as the bootstrap token: read
  mail, search, receive events, use MCP tools, and delete messages. It cannot
  manage credentials.
- Revoke by deleting the key ID. Requests using it immediately receive `401`.

## WebDAV users

```bash
# Generate a strong password (returned once)
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"username":"backup-agent"}' \
  http://localhost:8025/api/v1/admin/webdav-users

# Or supply a password of at least 12 characters
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"username":"backup-agent","password":"a-long-password"}' \
  http://localhost:8025/api/v1/admin/webdav-users
```

Response:

```json
{
  "username": "backup-agent",
  "password": "...",
  "generated": true,
  "created_at": "2026-11-08T05:00:00Z"
}
```

The password is hashed with bcrypt and never stored in plaintext. Successful
Basic-auth checks are cached in memory for five minutes so a WebDAV client's
many requests do not pay the bcrypt cost each time.

- Usernames are lower-cased.
- Creating a credential enables the `/dav/` surface even when
  `MAILVOID_WEBDAV_PASSWORD` is empty.
- The bootstrap WebDAV username is reserved while the environment password is
  set.
- Deleting a user invalidates cached auths and subsequent requests with that
  credential receive `401`.

## Events

| Event | Payload |
|-------|---------|
| `api_key.created` | `key_id`, `name`, `prefix`, `expires_at`, `username` |
| `api_key.deleted` | `key_id`, `name`, `prefix`, `username` |
| `webdav_user.created` | `username`, `generated`, `username_actor` |
| `webdav_user.deleted` | `username`, `username_actor` |

Plaintext tokens and passwords never appear in events.

## Security

- Managed API keys can read and delete all captured mail. Treat them as
  administrator-adjacent secrets.
- Managed WebDAV users can read every mailbox through `/dav/`; there is no
  per-mailbox authorization.
- Use HTTPS. Basic-auth WebDAV and bearer tokens are visible over plain HTTP.
- Rotate by creating a new credential and deleting the old one.
- Store tokens and generated passwords in a secret manager; they are shown only
  once.
