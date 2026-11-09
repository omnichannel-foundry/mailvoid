---
title: Eventos
weight: 30
---

# Eventos

Todos los eventos tienen el mismo sobre:

```json
{
  "id": "ev_9f3c1d2a7b4e6f80",
  "timestamp": "2026-11-08T01:28:24.083Z",
  "type": "smtp.rcpt_to",
  "severity": "info",
  "payload": {"accepted": true}
}
```

`payload` se omite cuando está vacío. `timestamp` está en UTC. El archivo JSONL almacena un objeto compacto por línea en el orden de emisión. Genji almacena los mismos campos más un `search_text` en minúsculas usado por el filtrado `?q=`.

## Severidades

| Severidad | Significado |
|-----------|-------------|
| `debug` | Reservada; actualmente ningún evento integrado la usa |
| `info` | Ciclo de vida normal |
| `warn` | Rechazos, descartes, fallos de análisis, inicios de sesión fallidos |
| `error` | Fallos de almacenamiento/listener/política |

## Sistema

| Tipo | Severidad | Payload |
|------|-----------|---------|
| `system.startup` | info | `version`, `smtp_ports` (array), `smtp_addresses` (array), `smtp_services` (array), `hostname`, `scheme`, `public_url`, `event_log_enabled`, `capture_enabled`, `log_enabled`, `log_dir`, `http_addr`, `domains` (array), `data_dir`, `db_path`, `event_log`, `admin_user`, `started_at` (RFC 3339), `max_message` (bytes) |
| `system.capture` | info | `enabled` (bool), `username`, `remote_ip` |
| `system.shutdown` | info | `version`, `reason` (`"shutdown"`) |

## Ciclo de vida SMTP

El flujo JSONL es la pista de auditoría entre servicios. Cada servicio SMTP también escribe un log de texto tradicional separado (consulta [Configuración](/getting-started/configuration/)).

| Tipo | Severidad | Payload |
|------|-----------|---------|
| `smtp.connected` | info | `remote_ip`, `local_port`, `service`, `aliases` (array), `hostname` (nombre EHLO/HELO del cliente) |
| `smtp.mail_from` | info | `remote_ip`, `local_port`, `sender` |
| `smtp.rcpt_to` | info / warn | `remote_ip`, `local_port`, `recipient`, `domain`, `mailbox`, `accepted` (bool). Los rechazos añaden `reason`. |
| `smtp.data` | info | `remote_ip`, `local_port`, `size` (bytes), `recipients` (recuento) |
| `smtp.closed` | info | `remote_ip`, `local_port` |
| `smtp.error` | warn / error | Depende del contexto; consulta a continuación |

### Payloads de rechazo de `smtp.rcpt_to`

Dirección mal formada:

```json
{
  "remote_ip": "203.0.113.9",
  "local_port": 1025,
  "recipient": "not-an-address",
  "accepted": false,
  "reason": "invalid address"
}
```

Dominio no permitido:

```json
{
  "remote_ip": "203.0.113.9",
  "local_port": 1025,
  "recipient": "user@denied.test",
  "domain": "denied.test",
  "accepted": false,
  "reason": "domain not accepted"
}
```

### Payloads de `smtp.error`

| Situación | Campos |
|-----------|--------|
| Fallo del listener | `error`, `local_port` |
| Mensaje demasiado grande | `remote_ip`, `local_port`, `error` (`"message too large"`), `size` |
| Fallo de análisis MIME (el mensaje se almacena igualmente) | `remote_ip`, `local_port`, `error`, `stored:true` |
| Fallo de evaluación de política (el mensaje se conserva) | `remote_ip`, `local_port`, `recipient`, `error` |

## Mensajes

| Tipo | Severidad | Payload |
|------|-----------|---------|
| `message.received` | info | Metadatos del mensaje |
| `message.dropped` | warn | Metadatos del mensaje + `reason:"policy drop"` |
| `message.deleted` | info | Metadatos del mensaje + `reason` + campos de actor |
| `message.error` | error | Específico del fallo |

### Campos de metadatos del mensaje

`message.received` y `message.dropped` incluyen:

| Campo | Descripción |
|-------|-------------|
| `message_id` | ID del mensaje almacenado |
| `remote_ip` | IP de origen |
| `local_port` | Puerto de recepción |
| `domain` | Dominio del destinatario |
| `mailbox` | Parte local del destinatario |
| `recipient` | Destinatario completo |
| `sender` | Remitente del sobre |
| `subject` | Asunto decodificado |
| `size` | Tamaño sin procesar en bytes |
| `action` | `keep`, `ttl` o `drop` |
| `policy_id` | ID de la política coincidente, o vacío |
| `policy` | Nombre de la política coincidente, o vacío |
| `expires_at` | Marca de tiempo RFC 3339 para mensajes con TTL |
| `reason` | Presente en los descartes (`"policy drop"`) |

### Variantes de `message.deleted`

| Origen | Campos adicionales |
|--------|--------------------|
| Barrido de TTL | `reason:"ttl expiry"`, `policy` (nombre coincidente) |
| Eliminación desde la consola | `reason:"manual deletion"`, `username` |
| Eliminación vía API | `reason:"api deletion"`, `username` |
| Eliminación vía MCP | `reason:"mcp deletion"`, `username:"mcp"` |

Todas las variantes incluyen `message_id`, `recipient`, `mailbox`, `domain`, `local_port`, `remote_ip`, `subject` y `action`.

### `message.error`

| Situación | Campos |
|-----------|--------|
| Fallo de almacenamiento SMTP | `remote_ip`, `local_port`, `recipient`, `error`; el cliente recibe `451` |
| Fallo del barrido de TTL | `error`, `reason:"ttl sweep"` |

## Políticas

| Tipo | Severidad | Payload |
|------|-----------|---------|
| `policy.matched` | info | `remote_ip`, `local_port`, `recipient`, `policy_id`, `policy_name`, `field`, `action` |
| `policy.created` | info | `policy_id`, `name`, `field`, `pattern`, `action`, `ttl_seconds`, `enabled`, `priority`, `username` |
| `policy.updated` | info | Igual que created (usado para ediciones, activaciones/desactivaciones y actualizaciones de la API) |
| `policy.deleted` | info | Igual que created |
| `policy.reordered` | info | Consola: `policy_id`, `direction` (`up`/`down`), `username`; API: `ids` (array ordenado), `username` |

`policy.matched` se emite una vez por destinatario cuando una regla coincide. No incluye el valor de texto coincidente.

## Credenciales

| Tipo | Severidad | Payload |
|------|-----------|---------|
| `api_key.created` | info | `key_id`, `name`, `prefix`, `expires_at`, `username` |
| `api_key.deleted` | info | `key_id`, `name`, `prefix`, `username` |
| `webdav_user.created` | info | `username`, `generated`, `username_actor` |
| `webdav_user.deleted` | info | `username`, `username_actor` |

Los tokens y las contraseñas nunca se incluyen.

## Búsqueda

| Tipo | Severidad | Payload |
|------|-----------|---------|
| `search.error` | error | `error` |

Un fallo de indexación de búsqueda no hace fallar la captura del mensaje; el mensaje permanece en Genji y el índice puede quedar desactualizado hasta una reconstrucción.

## Autenticación

| Tipo | Severidad | Payload |
|------|-----------|---------|
| `auth.login.success` | info | `username`, `remote_ip`, `user_agent` |
| `auth.login.failed` | warn | `username` (intentado), `remote_ip`, `user_agent` |
| `auth.logout` | info | `username`, `remote_ip` |
| `auth.password.changed` | info | `username`, `remote_ip` |

Las contraseñas nunca se incluyen en los eventos.

## Orden y retención

- Genji y los listados de la API devuelven los eventos del más reciente al más antiguo por marca de tiempo.
- El archivo JSONL conserva el orden de emisión.
- El búfer circular en memoria conserva los últimos 256 eventos.
- No hay retención ni compactación de eventos. La tabla y el archivo JSONL crecen durante toda la vida de la implementación a menos que los archives externamente.
