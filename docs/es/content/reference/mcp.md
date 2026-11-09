---
title: Servidor MCP
weight: 25
---

# Servidor MCP

MailVoid expone un servidor Model Context Protocol orientado a lectura en `/mcp`
para que los agentes puedan verificar la entrega, inspeccionar mensajes y leer el
rastro de auditoría sin construir solicitudes REST.

## Transporte

- **Streamable HTTP** en `/mcp` (`POST` JSON-RPC; el servidor devuelve respuestas
  JSON en lugar de flujos SSE).
- **Sin estado**: no se requiere continuidad de `Mcp-Session-Id`.
- El servidor MCP se identifica como `mailvoid` con la versión de compilación de
  MailVoid.

## Autenticación

`/mcp` usa la misma autenticación para agentes que `/api/v1`:

- `Authorization: Bearer <MAILVOID_API_TOKEN>`; o
- la cookie `mailvoid_session`.

Configura un token antes de exponer el endpoint:

```bash
MAILVOID_API_TOKEN='replace-with-a-long-random-token' ./mailvoid
```

Si no se configura ningún token, solo un cliente autenticado por navegador o
sesión puede usar MCP. Consulta [Configuración](/getting-started/configuration/).

## Configuración del cliente

La forma exacta depende del cliente MCP. Para los clientes que aceptan un servidor
Streamable HTTP remoto, los campos esenciales son la URL y la cabecera bearer:

```json
{
  "mcpServers": {
    "mailvoid": {
      "url": "http://localhost:8025/mcp",
      "headers": {
        "Authorization": "Bearer replace-with-your-token"
      }
    }
  }
}
```

Algunos clientes llaman al transporte `type: "http"` o requieren un objeto
`headers` separado. Consulta la documentación de tu cliente; la URL y la cabecera
son las partes que MailVoid requiere.

## Herramientas

| Herramienta | Propósito |
|-------------|-----------|
| `list_mailboxes` | Listar buzones con recuentos y metadatos del último mensaje |
| `check_mailbox` | Comprobar la entrega de un buzón y devolver los mensajes recientes |
| `wait_for_email` | Esperar un mensaje nuevo, con un tiempo de espera |
| `get_message` | Leer un mensaje, los cuerpos decodificados, las cabeceras y el contenido en bruto opcional |
| `search_messages` | Consultar mensajes con los filtros REST |
| `list_events` | Leer eventos de auditoría por tipo, gravedad o texto de búsqueda |
| `list_policies` | Leer las reglas de retención en orden |
| `get_stats` | Leer recuentos de captura, versión y tiempo de actividad |
| `delete_message` | Eliminar permanentemente un mensaje y emitir un evento de auditoría |

### `check_mailbox`

Entrada:

```json
{"address": "inbox@example.test", "since": "2026-11-01T00:00:00Z", "limit": 10}
```

Salida:

```json
{
  "found": true,
  "mailbox": {
    "address": "inbox@example.test",
    "mailbox": "inbox",
    "domain": "example.test",
    "message_count": 2,
    "latest_subject": "Welcome",
    "latest_received_at": "2026-11-08T01:28:24.083Z"
  },
  "messages": []
}
```

`found` es `true` cuando el buzón tiene al menos un mensaje capturado después de
`since` (o de todos los tiempos cuando se omite `since`).

### `wait_for_email`

Entrada:

```json
{
  "address": "inbox@example.test",
  "since": "2026-11-08T01:00:00Z",
  "timeout_seconds": 30
}
```

El tiempo de espera se limita a 1–60 segundos. Al agotarse, la herramienta tiene
éxito con `{"found": false, "waited_seconds": ...}`. Cuando hay entrega, devuelve
un resumen del mensaje.

### `get_message`

```json
{"id": "msg_81997d51ccdda1a5", "include_raw": true}
```

`include_raw` añade el mensaje RFC 5322 completo al resultado. Sin él, la
herramienta devuelve metadatos, cabeceras, cabeceras en bruto y cuerpos text/HTML
decodificados.

### `search_messages`

Acepta `q`, `domain`, `mailbox`, `recipient`, `sender`, `action`, `port`, `since`,
`until`, `limit`, `offset` y `sort` (`relevance` o `received_at`). Con el índice de
búsqueda habilitado, `q` es una consulta de texto completo Bleve sobre asunto,
cuerpo decodificado, remitente, destinatario, buzón, dominio y cabeceras. Los
resultados incluyen `messages`, `count`, `backend` (`bleve` o `fallback`) y
`highlights` opcional. Consulta
[Búsqueda de texto completo](/reference/search/).

### `delete_message`

Elimina permanentemente un mensaje y emite `message.deleted` con
`reason: "mcp deletion"` y `username: "mcp"`. Trata esta herramienta como
destructiva.

## Ejemplo de inicialización JSON-RPC

Los clientes MCP realizan la inicialización automáticamente. La forma de la
solicitud en bruto es:

```bash
curl -s http://localhost:8025/mcp \
  -H 'Authorization: Bearer replace-with-your-token' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
          "protocolVersion": "2025-06-18",
          "capabilities": {},
          "clientInfo": {"name": "curl", "version": "1"}
        }
      }'
```

Luego usa `tools/list` y `tools/call` según lo define la especificación MCP. Se
recomienda un cliente del SDK de MCP en lugar de JSON-RPC escrito a mano.

## Seguridad

- El endpoint MCP puede leer todos los mensajes capturados y eliminarlos. Protege
  el token como una contraseña de administrador.
- Prefiere HTTPS mediante un proxy inverso; los tokens bearer sobre HTTP sin
  cifrar pueden observarse en la red.
- El servidor no tiene estado, por lo que no hay una sesión MCP que revocar.
  Rota el token de la API y reinicia MailVoid para invalidar los tokens antiguos.
- Las llamadas a herramientas no se registran por separado en la auditoría. Las
  operaciones que realizan (por ejemplo, la eliminación) emiten sus eventos
  normales.

## Qué no expone MCP

- La creación o edición de políticas. Eso sigue estando disponible mediante la
  consola y la API REST.
- La transmisión de eventos. Usa `/ws/events`, `/api/events/stream` o la consola
  para datos en vivo.
- La descarga del archivo `.eml` en bruto como herramienta separada. Usa
  `get_message` con `include_raw: true`, o las rutas de descarga REST y WebDAV.
