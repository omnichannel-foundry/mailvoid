---
title: API HTTP
weight: 20
---

# API HTTP

> [!NOTE]
> La API máquina canónica es **`/api/v1`**, documentada en
> [OpenAPI](/reference/openapi/) y consumida por [MCP](/reference/mcp/). Añade
> autenticación con token bearer, resúmenes de buzón, esperas de entrega y descarga
> de `.eml`. Las rutas `/api` de esta página siguen disponibles para la consola y
> para los clientes existentes autenticados por sesión.

Todos los endpoints los sirve el mismo proceso Fiber que la consola. Las respuestas JSON usan UTF-8 y marcas de tiempo RFC 3339.

## Autenticación

Todos los endpoints excepto `GET /healthz`, `GET /static/*` y las rutas de inicio de sesión requieren una cookie de sesión válida. Obtén una enviando el formulario de inicio de sesión:

```bash
curl -s -c cookies.txt \
  -d 'username=admin&password=change-me-please' \
  http://localhost:8025/login

curl -s -b cookies.txt http://localhost:8025/api/messages
```

Las solicitudes a la API sin autenticar devuelven:

```http
HTTP/1.1 401 Unauthorized
Content-Type: application/json

{"error":"authentication required"}
```

Estas rutas heredadas `/api` requieren la cookie de sesión. Los tokens bearer solo se aceptan en `/api/v1` y `/mcp`.

### Endpoints de sesión

| Método | Ruta | Cuerpo | Resultado |
|--------|------|--------|-----------|
| `GET` | `/login` | — | Página de inicio de sesión (redirige al inicio si ya se inició sesión) |
| `POST` | `/login` | form: `username`, `password`, `next` opcional | `302` + `Set-Cookie` si tiene éxito; página de inicio de sesión `401` si falla |
| `POST` | `/logout` | — | Destruye la sesión, `302` a `/login` |

`next` solo se acepta cuando es una ruta local que comienza por `/` y no por `//`. Las redirecciones externas se rechazan.

## Errores

Todos los errores de la API usan:

```json
{"error":"human-readable message"}
```

| Estado | Significado |
|--------|-------------|
| `400` | JSON inválido, regex inválida, campo/acción desconocidos, lista de reordenamiento incorrecta |
| `401` | Sesión ausente o expirada |
| `404` | Mensaje o política no encontrados |
| `500` | Fallo de almacenamiento o interno |

## Paginación y filtros

Los endpoints de listado aceptan:

| Parámetro | Valor predeterminado | Límites |
|-----------|----------------------|---------|
| `page` | `1` | Basado en 1 |
| `limit` | `50` | 1–200 |

Las respuestas incluyen `count` (coincidencias totales), `limit` y `offset`.

## Mensajes

### `GET /api/messages`

Lista los metadatos de los mensajes, del más reciente al más antiguo. Los cuerpos y el contenido sin procesar se omiten del listado; para obtenerlos, consulta el endpoint de detalle.

Parámetros de consulta:

| Parámetro | Descripción |
|-----------|-------------|
| `q` | Subcadena sin distinción de mayúsculas y minúsculas sobre remitente, destinatario, buzón, dominio, asunto e IP remota |
| `domain` | Dominio exacto |
| `mailbox` | Parte local exacta |
| `action` | `keep` o `ttl` |
| `port` | Puerto de recepción exacto |
| `page`, `limit` | Paginación |

```bash
curl -s -b cookies.txt \
  'http://localhost:8025/api/messages?q=invoice&port=1025&limit=20'
```

```json
{
  "messages": [
    {
      "id": "msg_81997d51ccdda1a5",
      "received_at": "2026-11-08T01:28:24.083Z",
      "remote_ip": "203.0.113.9",
      "local_port": 1025,
      "domain": "example.test",
      "mailbox": "inbox",
      "recipient": "inbox@example.test",
      "sender": "sender@example.org",
      "subject": "Invoice 42",
      "size": 552,
      "action": "ttl",
      "policy_id": "pol_a89e827c828431fe",
      "policy_name": "expire invoices",
      "expires_at": "2026-11-09T01:28:24Z"
    }
  ],
  "count": 1,
  "limit": 20,
  "offset": 0
}
```

`policy_id`, `policy_name` y `expires_at` se omiten cuando están vacíos.

### `GET /api/messages/:id`

Devuelve el registro completo del mensaje.

```json
{
  "id": "msg_81997d51ccdda1a5",
  "received_at": "2026-11-08T01:28:24.083Z",
  "remote_ip": "203.0.113.9",
  "local_port": 1025,
  "domain": "example.test",
  "mailbox": "inbox",
  "recipient": "inbox@example.test",
  "sender": "sender@example.org",
  "subject": "Invoice 42",
  "headers": {
    "Content-Type": ["multipart/alternative; boundary=..."],
    "Subject": ["Invoice 42"]
  },
  "raw_headers": "Content-Type: ...\r\nSubject: Invoice 42\r\n",
  "raw": "Content-Type: ...\r\n\r\n--boundary...",
  "body_text": "plain text body",
  "body_html": "<p>HTML body</p>",
  "size": 552,
  "policy_id": "pol_a89e827c828431fe",
  "policy_name": "expire invoices",
  "action": "ttl",
  "expires_at": "2026-11-09T01:28:24Z",
  "deleted": false
}
```

### `DELETE /api/messages/:id`

Elimina un mensaje de forma permanente y emite `message.deleted` con el motivo `api deletion`.

```json
{"deleted": true, "id": "msg_81997d51ccdda1a5"}
```

## Eventos

### `GET /api/events`

Lista los eventos del más reciente al más antiguo.

| Parámetro | Descripción |
|-----------|-------------|
| `type` | Tipo de evento exacto, p. ej. `message.dropped` |
| `severity` | `debug`, `info`, `warn` o `error` |
| `q` | Subcadena sin distinción de mayúsculas y minúsculas sobre tipo, severidad y JSON del payload |
| `page`, `limit` | Paginación |

```json
{
  "events": [
    {
      "id": "ev_9f3c1d2a7b4e6f80",
      "timestamp": "2026-11-08T01:28:24.083Z",
      "type": "message.received",
      "severity": "info",
      "payload": {
        "message_id": "msg_81997d51ccdda1a5",
        "remote_ip": "203.0.113.9",
        "local_port": 1025,
        "domain": "example.test",
        "mailbox": "inbox",
        "recipient": "inbox@example.test",
        "sender": "sender@example.org",
        "subject": "Invoice 42",
        "size": 552,
        "action": "ttl"
      }
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### `GET /api/events/stream`

Server-Sent Events. El primer frame es `event: ready`; los frames siguientes usan el tipo de evento como nombre de evento SSE. Cada 20 segundos se envía un ping de comentario.

```text
event: ready
data: {"ok":true}

event: smtp.connected
data: {"id":"ev_...","timestamp":"...","type":"smtp.connected","severity":"info","payload":{...}}
```

El stream no admite reproducción. Usa `GET /api/events` para el historial.

### `GET /ws/events`

Actualización a WebSocket. Requiere la cookie de sesión en el handshake. Cada evento es un frame de texto JSON. El servidor envía un ping cada 25 segundos.

```javascript
const ws = new WebSocket("ws://localhost:8025/ws/events");
ws.onmessage = (message) => {
  const event = JSON.parse(message.data);
  console.log(event.type, event.payload);
};
```

Los clientes de navegador envían las cookies automáticamente. Los clientes que no son navegadores deben establecer la cabecera `Cookie`.

## Políticas

### `GET /api/policies`

```json
{
  "policies": [
    {
      "id": "pol_366af449b712cc77",
      "name": "drop spam",
      "field": "sender",
      "pattern": "@spam\\.test$",
      "action": "drop",
      "ttl_seconds": 0,
      "priority": 0,
      "enabled": true,
      "created_at": "2026-11-08T01:28:30.373Z",
      "updated_at": "2026-11-08T01:28:30.373Z"
    }
  ],
  "count": 1
}
```

### `POST /api/policies`

Crea una política. `enabled` toma el valor `true` por defecto cuando se omite. `priority` se añade después de la última regla actual cuando se omite o es cero.

```bash
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{
        "name": "drop spam",
        "field": "sender",
        "pattern": "@spam\\.test$",
        "action": "drop",
        "enabled": true
      }' \
  http://localhost:8025/api/policies
```

Devuelve `201 Created` con la política almacenada.

Validación:

- `name` obligatorio;
- `field` debe ser un campo conocido;
- `action` debe ser `keep`, `ttl` o `drop`;
- `pattern` debe compilar y tener como máximo 2048 caracteres;
- `ttl_seconds > 0` es obligatorio para `ttl` y se normaliza a `0` en caso contrario.

### `GET /api/policies/:id`

Devuelve una política o `404`.

### `PUT /api/policies/:id`

Actualización de reemplazo completo de los campos de texto de la regla. Las cadenas ausentes y `ttl_seconds` pasan a valores cero, lo que comúnmente hace fallar la validación. Si faltan `enabled` y `priority`, se conservan los valores existentes. `id` y `created_at` siempre se conservan.

### `DELETE /api/policies/:id`

```json
{"deleted": true, "id": "pol_366af449b712cc77"}
```

### `POST /api/policies/reorder`

El array `ids` debe contener todas las políticas exactamente una vez. Las prioridades pasan a ser el índice del array (`0` primero).

```bash
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"ids":["pol_b","pol_a","pol_c"]}' \
  http://localhost:8025/api/policies/reorder
```

```json
{"reordered": true}
```

### `POST /api/policies/test`

Evalúa un patrón contra un valor de muestra sin guardar una regla.

```bash
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"field":"subject","pattern":"^invoice","value":"Invoice 42"}' \
  http://localhost:8025/api/policies/test
```

```json
{"field":"subject","matched":true,"pattern":"^invoice","value":"Invoice 42"}
```

Los campos desconocidos y las regex inválidas devuelven `400`.

## Estadísticas

### `GET /api/stats`

```json
{
  "messages": 42,
  "messages_24h": 7,
  "policies": 3,
  "events": 512,
  "version": "v1.2.3",
  "uptime": "1h2m3s"
}
```

## Estado de salud

### `GET /healthz`

Público, sin autenticación.

```json
{"status":"ok","version":"v1.2.3","uptime":"1h2m3s"}
```

## Recursos estáticos

`GET /static/*` sirve el CSS de DaisyUI, HTMX, Cash, el CSS local y el JavaScript local integrados, con una caché de una hora y `X-Content-Type-Options: nosniff`.
