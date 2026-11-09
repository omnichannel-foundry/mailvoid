---
title: Reverse proxy
weight: 30
---

# Reverse proxy

Terminate TLS at a reverse proxy and keep MailVoid on loopback:

```bash
MAILVOID_HTTP_ADDR=127.0.0.1:8025
MAILVOID_COOKIE_SECURE=true
```

The proxy must support:

- WebSocket upgrade on `/ws/events`;
- unbuffered streaming on `/api/events/stream`;
- long read timeouts for live connections.

## Nginx

Define the upgrade map once in the `http` block:

```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}
```

Site configuration:

```nginx
server {
    listen 443 ssl http2;
    server_name mail.example.test;

    ssl_certificate     /etc/letsencrypt/live/mail.example.test/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/mail.example.test/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8025;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade           $http_upgrade;
        proxy_set_header Connection        $connection_upgrade;
    }

    # Server-Sent Events: never buffer
    location = /api/events/stream {
        proxy_pass http://127.0.0.1:8025;
        proxy_http_version 1.1;
        proxy_set_header Host            $host;
        proxy_set_header Connection      '';
        proxy_buffering                  off;
        proxy_cache                      off;
        gzip                             off;
        proxy_read_timeout               1h;
        proxy_send_timeout               1h;
    }

    # WebSocket
    location = /ws/events {
        proxy_pass http://127.0.0.1:8025;
        proxy_http_version 1.1;
        proxy_set_header Host       $host;
        proxy_set_header Upgrade    $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_read_timeout          1h;
    }
}
```

Add an HTTP-to-HTTPS redirect server and use your normal certificate workflow.

## Caddy

```caddyfile
mail.example.test {
    reverse_proxy 127.0.0.1:8025 {
        header_up X-Forwarded-For {remote_host}
        # Disable response buffering for SSE.
        flush_interval -1
    }
}
```

Caddy handles WebSocket upgrades automatically. `flush_interval -1` is the
important setting for Server-Sent Events.

## Client IP in login events

Fiber is not configured with a trusted proxy header in this release. Login and
logout events therefore record the TCP peer seen by MailVoid, which is usually
the proxy address. SMTP events always record the real SMTP client address.

If accurate HTTP client IPs matter, extend the Fiber configuration with a
`ProxyHeader` (for example `X-Forwarded-For`) and only enable it when the
listener is unreachable except through the trusted proxy.

## Agent and WebDAV routes

- `/api/v1` and `/mcp` are normal HTTP endpoints. Forward them with the same
  proxy as the console; keep the `Authorization` header intact.
- `/mcp` uses JSON responses (stateless Streamable HTTP), so it does not need
  SSE-specific buffering settings. Enable WebSocket/SSE settings only for
  `/ws/events` and `/api/events/stream`.
- `/dav/` uses HTTP Basic authentication. Terminate HTTPS at the proxy and do
  not strip the `Authorization` header. WebDAV clients may send `PROPFIND`,
  `OPTIONS`, `GET`, and `HEAD`; proxy configurations that allow only GET/POST
  will break listings.
- Proxy authentication (for example, an SSO layer) can sit in front of `/dav/`
  and `/api/v1`, but it does not replace MailVoid's own bearer/Basic checks.

## SMTP is not proxied

Do not send SMTP through an HTTP reverse proxy. Expose the SMTP ports directly to
the clients, or route them through a TCP load balancer or SMTP-aware gateway.

## Health checks

Point a load balancer or monitoring probe at `GET /healthz` on the loopback
listener. Do not health-check through the public hostname if the dashboard is
meant to stay private.
