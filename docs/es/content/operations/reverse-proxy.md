---
title: Proxy inverso
weight: 30
---

# Proxy inverso

Termina TLS en un proxy inverso y mantén MailVoid en loopback:

```bash
MAILVOID_HTTP_ADDR=127.0.0.1:8025
MAILVOID_COOKIE_SECURE=true
```

El proxy debe admitir:

- actualización de WebSocket en `/ws/events`;
- streaming sin búfer en `/api/events/stream`;
- tiempos de lectura largos para conexiones en vivo.

## Nginx

Define el mapa de actualización una vez en el bloque `http`:

```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}
```

Configuración del sitio:

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

Añade un servidor de redirección de HTTP a HTTPS y usa tu flujo habitual de
certificados.

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

Caddy gestiona las actualizaciones de WebSocket automáticamente.
`flush_interval -1` es el ajuste importante para Server-Sent Events.

## IP del cliente en los eventos de inicio de sesión

Fiber no está configurado con una cabecera de proxy de confianza en esta versión.
Por lo tanto, los eventos de inicio y cierre de sesión registran el par TCP que ve
MailVoid, que normalmente es la dirección del proxy. Los eventos SMTP siempre
registran la dirección real del cliente SMTP.

Si importan las IP reales de los clientes HTTP, amplía la configuración de Fiber
con un `ProxyHeader` (por ejemplo, `X-Forwarded-For`) y actívalo solo cuando el
listener no sea accesible salvo a través del proxy de confianza.

## Rutas de agente y WebDAV

- `/api/v1` y `/mcp` son endpoints HTTP normales. Reenvíalos con el mismo proxy
  que la consola; mantén intacta la cabecera `Authorization`.
- `/mcp` usa respuestas JSON (Streamable HTTP sin estado), así que no necesita
  ajustes de búfer específicos de SSE. Activa los ajustes de WebSocket/SSE solo
  para `/ws/events` y `/api/events/stream`.
- `/dav/` usa autenticación HTTP Basic. Termina HTTPS en el proxy y no elimines
  la cabecera `Authorization`. Los clientes WebDAV pueden enviar `PROPFIND`,
  `OPTIONS`, `GET` y `HEAD`; las configuraciones de proxy que solo permiten
  GET/POST romperán los listados.
- La autenticación en el proxy (por ejemplo, una capa SSO) puede situarse delante
  de `/dav/` y `/api/v1`, pero no sustituye las propias comprobaciones
  bearer/Basic de MailVoid.

## SMTP no se proxifica

No envíes SMTP a través de un proxy inverso HTTP. Expón los puertos SMTP
directamente a los clientes, o enrútalos a través de un balanceador de carga TCP
o una pasarela que entienda SMTP.

## Comprobaciones de estado

Apunta un balanceador de carga o una sonda de monitorización a `GET /healthz` en
el listener de loopback. No hagas comprobaciones de estado a través del nombre de
host público si el panel de control debe permanecer privado.
