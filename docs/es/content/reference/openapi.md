---
title: OpenAPI
weight: 15
---

# OpenAPI

La API máquina canónica de MailVoid está versionada bajo `/api/v1`. Se describe
mediante un documento OpenAPI 3.1.0 embebido en el binario y servido en dos rutas:

| Ruta | Tipo de contenido |
|------|-------------------|
| `/api/v1/openapi.json` | `application/json` |
| `/api/v1/openapi.yaml` | `application/yaml` |

Ambas son públicas para que las herramientas puedan descubrir la API sin
credenciales. Las operaciones en sí requieren autenticación.

## Autenticación

La API v1 acepta cualquiera de estas formas:

- `Authorization: Bearer <MAILVOID_API_TOKEN>` para agentes y automatización;
- la cookie `mailvoid_session` creada por `POST /login` para navegadores y
  herramientas de consola.

Si no se configura un token de API, las solicitudes bearer se rechazan y solo
funciona la cookie de sesión. Consulta
[Configuración](/getting-started/configuration/).

## Mapa de endpoints

| Método | Ruta | Propósito |
|--------|------|-----------|
| `GET` | `/api/v1` | Metadatos y enlaces de la API |
| `GET` | `/api/v1/mailboxes` | Resúmenes de buzones con recuentos |
| `GET` | `/api/v1/mailboxes/{address}` | Resumen de un buzón |
| `GET` | `/api/v1/mailboxes/{address}/messages` | Mensajes de un buzón |
| `GET` | `/api/v1/mailboxes/{address}/wait` | Long-poll para un mensaje nuevo |
| `GET` | `/api/v1/messages` | Listar/filtrar mensajes (subcadena de metadatos `q`) |
| `GET` | `/api/v1/search` | Búsqueda de texto completo Bleve con ranking |
| `GET` | `/api/v1/messages/{id}` | Mensaje completo y cuerpos |
| `GET` | `/api/v1/messages/{id}/eml` | Descarga del mensaje en bruto |
| `DELETE` | `/api/v1/messages/{id}` | Eliminar un mensaje |
| `GET` | `/api/v1/events` | Eventos de auditoría |
| `GET` | `/api/v1/policies` | Listar políticas |
| `POST` | `/api/v1/policies` | Crear una política |
| `GET` | `/api/v1/policies/{id}` | Obtener una política |
| `PUT` | `/api/v1/policies/{id}` | Actualizar una política |
| `DELETE` | `/api/v1/policies/{id}` | Eliminar una política |
| `POST` | `/api/v1/policies/reorder` | Reordenar políticas |
| `POST` | `/api/v1/policies/test` | Probar una expresión regular |
| `GET` | `/api/v1/stats` | Estadísticas de captura |
| `GET/POST` | `/api/v1/admin/api-keys` | Listar/crear claves de API gestionadas |
| `DELETE` | `/api/v1/admin/api-keys/{id}` | Revocar una clave de API |
| `GET/POST` | `/api/v1/admin/webdav-users` | Listar/crear usuarios WebDAV |
| `DELETE` | `/api/v1/admin/webdav-users/{username}` | Eliminar un usuario WebDAV |

Los esquemas completos de solicitud y respuesta, los parámetros y las definiciones
de seguridad están en el documento servido. Esta página cubre los flujos de
trabajo comunes.

## Comprobar si llegó correo

```bash
TOKEN='your-api-token'

# All mailboxes
curl -s -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8025/api/v1/mailboxes'

# One mailbox
curl -s -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8025/api/v1/mailboxes/inbox@example.test'

# Recent messages in that mailbox
curl -s -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8025/api/v1/mailboxes/inbox@example.test/messages?limit=10'
```

Un resumen de buzón incluye `message_count`, `latest_message_id`,
`latest_subject`, `latest_sender` y `latest_received_at`.

## Esperar la entrega

Haz long-poll en lugar de dormir en un bucle de agente:

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8025/api/v1/mailboxes/inbox@example.test/wait?timeout=30s'
```

- `since` es un límite inferior RFC 3339 opcional. Si se omite, la espera comienza
  en la hora actual.
- `timeout` es una duración de Go, limitada a 1–60 segundos.
- Al agotarse el tiempo, la respuesta es `200` con
  `{"found":false,"waited_seconds":...}`.
- Cuando hay entrega, la respuesta contiene `found`, `waited_seconds` y un resumen
  del mensaje.

## Buscar mensajes

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8025/api/v1/messages?q=invoice&since=2026-11-01T00:00:00Z&limit=50'
```

Parámetros admitidos:

| Parámetro | Significado |
|-----------|-------------|
| `q` | Subcadena sobre metadatos, sin distinción de mayúsculas |
| `domain`, `mailbox`, `recipient` | Filtros de destino exactos |
| `action` | `keep`, `ttl` o `drop` |
| `port` | Puerto de recepción |
| `since`, `until` | Límites RFC 3339 |
| `limit` | 1–200, predeterminado 50 |
| `page` u `offset` | Paginación; `offset` tiene prioridad |

Las marcas de tiempo, puertos, acciones, límites u offsets no válidos devuelven
`400`.

## Descargar .eml

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  -o message.eml \
  'http://localhost:8025/api/v1/messages/msg_81997d51ccdda1a5/eml'
```

La respuesta es `message/rfc822` con un nombre de archivo adjunto derivado del
asunto y el ID del mensaje.

## `/api` heredada

Las rutas `/api` más antiguas siguen disponibles para la consola y los scripts
existentes. Usan los mismos manejadores de mensajes, eventos, políticas y
estadísticas, requieren la cookie de sesión (sin token bearer) y no incluyen los
endpoints de buzón, espera ni `.eml`. Las nuevas integraciones deberían usar
`/api/v1`.

## Herramientas

Carga `/api/v1/openapi.json` en cualquier cliente compatible con OpenAPI, colección
de Postman o generador de código. El documento usa componentes `$ref` locales y no
contiene referencias externas.
