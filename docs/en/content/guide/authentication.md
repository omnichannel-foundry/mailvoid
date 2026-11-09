---
title: Authentication
weight: 50
---

# Authentication

MailVoid uses a single administrator model: every authenticated account has full
access to messages, policies, events, settings, and the API. There are no roles
or per-mailbox permissions.

## Bootstrap administrator

The account is created on first start from:

- `MAILVOID_ADMIN_USER` (default `admin`; an explicitly empty value fails
  configuration validation);
- `MAILVOID_ADMIN_PASSWORD`.

If the password is empty, MailVoid generates a random 24-character URL-safe
password and prints it once during startup. If the account already exists, the
password is left unchanged unless `MAILVOID_ADMIN_RESET_PASSWORD=true`.

```bash
# first start with a known password
MAILVOID_ADMIN_PASSWORD='change-me-please' ./mailvoid

# later, intentionally reset it
MAILVOID_ADMIN_PASSWORD='new-password' \
MAILVOID_ADMIN_RESET_PASSWORD=true \
./mailvoid
```

Passwords are hashed with bcrypt at cost 12. Plaintext passwords are never
stored.

## Sessions

- Login sets an HTTP-only cookie named `mailvoid_session`.
- Cookie defaults: `SameSite=Lax`, path `/`, 12-hour expiry.
- The session ID is regenerated on successful login to prevent fixation.
- Sessions are held in memory. Restarting MailVoid signs everyone out.
- Set `MAILVOID_COOKIE_SECURE=true` when the dashboard is served over HTTPS.

## Signing in

Open `http://<host>:8025` and use the login form.

For scripts, keep a cookie jar:

```bash
curl -s -c cookies.txt \
  -d 'username=admin&password=change-me-please' \
  http://localhost:8025/login

curl -s -b cookies.txt http://localhost:8025/api/messages
```

A failed login returns HTTP `401` with the login page and emits
`auth.login.failed` containing the attempted username, remote IP, and user
agent. The password is never included in the event.

## Signing out

The top-right menu posts to `/logout`. The session is destroyed and
`auth.logout` is emitted.

## Changing the password

**Settings → Administrator password** requires the current password, a new
password of at least 8 characters, and confirmation. Success emits
`auth.password.changed` and redirects with a flash message.

There is no password-strength policy beyond the length check. Use a long,
unique password from a secret manager.

## Route protection

| Route | Authentication |
|-------|----------------|
| `/healthz` | Public |
| `/static/*` | Public |
| `GET/POST /login` | Public |
| `/` and every console page | Session required |
| `/api/*` | Session required |
| `/ws/events` | Session required |
| `/logout` | Session required |

Unauthenticated page requests redirect to `/login?next=<original-url>`.
Unauthenticated API and WebSocket requests receive HTTP `401` with
`{"error":"authentication required"}`.

The `next` parameter is validated to be a local absolute path; external redirects
are rejected.

## Agent credentials

For automation, configure `MAILVOID_API_TOKEN` and send:

```http
Authorization: Bearer <token>
```

This is accepted by `/api/v1` and `/mcp`. It is not accepted by the legacy
`/api` routes or the HTML console. If no token is configured, bearer requests
are rejected and agents must use a session cookie.

Managed API keys and WebDAV users can also be created and revoked at runtime
through the administrator API. See [Managed credentials](/reference/credentials/).

## WebDAV credentials

`/dav/` uses a separate Basic-auth pair:

- `MAILVOID_WEBDAV_USER` (default `mailvoid`);
- `MAILVOID_WEBDAV_PASSWORD` (empty disables WebDAV).

WebDAV credentials do not grant access to the console or API. See
[WebDAV](/operations/webdav/).

## Security notes

- Any logged-in account is an administrator. Treat the bootstrap credentials as
  root credentials for the service.
- Login rate limiting is not implemented. Put the dashboard behind a reverse
  proxy that can rate-limit, or restrict network access.
- CSRF tokens are not implemented. The session cookie uses `SameSite=Lax`, which
  blocks most cross-site form POSTs in modern browsers. Add a proxy-level control
  if your threat model requires defense in depth.
- There is no account management UI. Create additional administrators through
  the repository API only if you are extending the code; the shipped bootstrap
  flow manages a single account.
- Session cookies are marked `Secure` only when configured. Using
  `MAILVOID_COOKIE_SECURE=true` without HTTPS makes login fail because the
  browser will not send the cookie back.
