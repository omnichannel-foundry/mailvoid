---
title: Modelo de datos
weight: 40
---

# Modelo de datos

MailVoid almacena cuatro tipos de registros en Genji: mensajes, políticas, eventos y usuarios. Los tipos de Go viven en `internal/models`; la interfaz del repositorio vive en `internal/storage`.

## Mensajes

### Tabla `messages` de Genji

| Columna | Tipo | Notas |
|---------|------|-------|
| `id` | TEXT PK | `msg_` + 16 caracteres hexadecimales |
| `received_at` | INT | Milisegundos Unix, UTC |
| `remote_ip` | TEXT | Dirección del par TCP |
| `local_port` | INT | Puerto SMTP de recepción |
| `domain` | TEXT | Dominio del destinatario en minúsculas |
| `mailbox` | TEXT | Parte local en minúsculas |
| `recipient` | TEXT | `mailbox@domain` en minúsculas |
| `sender` | TEXT | Remitente de sobre en minúsculas; vacío para `<>` |
| `subject` | TEXT | Asunto decodificado |
| `headers` | TEXT | Objeto JSON: `map[string][]string` |
| `raw_headers` | TEXT | Bloque de cabeceras tal como se recibió |
| `raw` | TEXT | Mensaje sin procesar completo |
| `body_text` | TEXT | `text/plain` decodificado |
| `body_html` | TEXT | `text/html` decodificado |
| `size_bytes` | INT | Longitud en bytes sin procesar |
| `policy_id` | TEXT | Política coincidente, o vacío |
| `policy_name` | TEXT | Nombre de la política coincidente, o vacío |
| `policy_action` | TEXT | `keep`, `ttl` o `drop` |
| `expires_at` | INT | Milisegundos Unix; `0` significa sin expiración |
| `deleted` | INT | `0`/`1`; reservado. La eliminación actual es física. |
| `deleted_at` | INT | Milisegundos Unix; reservado |
| `search_text` | TEXT | Concatenación en minúsculas usada por `?q=` |

### Representación de la API

La API JSON expone el mismo contenido con nombres en `snake_case`. Las respuestas de listado omiten `headers`, `raw_headers`, `raw`, `body_text` y `body_html`. Las respuestas de detalle incluyen todos los campos. `deleted_at` y los campos de política/expiración se omiten cuando están vacíos.

### Eliminación

Los mensajes se eliminan físicamente. No existe papelera ni flujo de eliminación lógica; las columnas `deleted` y `deleted_at` están reservadas para una implementación futura y permanecen a cero para los mensajes almacenados.

## Políticas

### Tabla `policies` de Genji

| Columna | Tipo | Notas |
|---------|------|-------|
| `id` | TEXT PK | `pol_` + 16 caracteres hexadecimales |
| `name` | TEXT | Etiqueta legible |
| `field_name` | TEXT | Campo objetivo |
| `pattern` | TEXT | Expresión RE2 de Go |
| `policy_action` | TEXT | `keep`, `ttl` o `drop` |
| `ttl_seconds` | INT | Obligatorio para `ttl`; en caso contrario, 0 |
| `priority` | INT | Orden de evaluación ascendente, comenzando en 0 |
| `enabled` | INT | `0`/`1` |
| `created_at` | INT | Milisegundos Unix |
| `updated_at` | INT | Milisegundos Unix |

### Representación de la API

```json
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
```

Los nombres de la API difieren de los nombres de las columnas de almacenamiento: `field_name` pasa a ser `field`, y `policy_action` pasa a ser `action`.

## Eventos

### Tabla `events` de Genji

| Columna | Tipo | Notas |
|---------|------|-------|
| `id` | TEXT PK | `ev_` + 16 caracteres hexadecimales |
| `event_time` | INT | Milisegundos Unix, UTC |
| `event_type` | TEXT | Tipo con puntos |
| `severity` | TEXT | `debug`, `info`, `warn`, `error` |
| `payload` | TEXT | Objeto JSON |
| `search_text` | TEXT | Tipo + severidad + payload en minúsculas |

### Representación de la API

```json
{
  "id": "ev_9f3c1d2a7b4e6f80",
  "timestamp": "2026-11-08T01:28:24.083Z",
  "type": "message.received",
  "severity": "info",
  "payload": {"message_id": "msg_81997d51ccdda1a5"}
}
```

Los nombres de las columnas de almacenamiento son `event_time` y `event_type`; los nombres JSON son `timestamp` y `type`.

## Usuarios

### Tabla `users` de Genji

| Columna | Tipo | Notas |
|---------|------|-------|
| `username` | TEXT PK | En minúsculas |
| `password_hash` | TEXT | Hash bcrypt (coste 12) |
| `created_at` | INT | Milisegundos Unix |

La API nunca devuelve contraseñas y estas no están presentes en los payloads de eventos. La aplicación distribuida expone una única cuenta de arranque y un flujo de cambio de contraseña en lugar de endpoints de administración de usuarios.

## Identificadores

| Prefijo | Registro |
|---------|----------|
| `msg_` | Mensaje |
| `pol_` | Política |
| `ev_` | Evento |

Los identificadores se generan con `crypto/rand` y se codifican en hexadecimal. Son seguros para URL y distinguen mayúsculas de minúsculas.

## Gestión del tiempo

- Todas las horas persistidas son UTC.
- El almacenamiento usa milisegundos Unix; `0` representa "ausente" para las horas opcionales (`expires_at`, `deleted_at`).
- JSON usa RFC 3339 con sufijo `Z`.
- La consola muestra marcas de tiempo UTC; el cliente de eventos en vivo formatea la hora de llegada en la zona horaria local del navegador.

## Texto de búsqueda

`search_text` es una columna de conveniencia solo para el almacenamiento:

- mensaje: remitente, destinatario, buzón, dominio, asunto e IP remota en minúsculas;
- evento: tipo, severidad y payload serializado en minúsculas.

Los filtros convierten la consulta del usuario a minúsculas y usan `LIKE '%query%'` contra esta columna. La búsqueda no incluye los cuerpos de los mensajes, las cabeceras sin procesar ni el mensaje sin procesar.
