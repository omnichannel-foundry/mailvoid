---
title: Flujo de eventos
weight: 40
---

# Flujo de eventos

MailVoid registra cada acción significativa del ciclo de vida como un evento
estructurado. Los eventos se persisten en dos lugares y se difunden a los
consumidores en vivo.

## Forma del evento

```json
{
  "id": "ev_9f3c1d2a7b4e6f80",
  "timestamp": "2026-11-08T01:28:24.083Z",
  "type": "message.received",
  "severity": "info",
  "payload": {
    "message_id": "msg_81997d51ccdda1a5",
    "remote_ip": "127.0.0.1",
    "local_port": 12525,
    "domain": "example.test",
    "mailbox": "inbox",
    "recipient": "inbox@example.test",
    "sender": "sender@example.org",
    "subject": "Hello from port 12525",
    "size": 552,
    "action": "keep"
  }
}
```

| Campo | Descripción |
|-------|-------------|
| `id` | `ev_` más 16 caracteres hexadecimales aleatorios |
| `timestamp` | Hora UTC, RFC 3339 en JSON |
| `type` | Tipo de evento con puntos, como `smtp.rcpt_to` |
| `severity` | `debug`, `info`, `warn` o `error` |
| `payload` | Objeto JSON específico del evento; se omite cuando está vacío |

## Persistencia

- **Archivo JSONL** — un evento por línea en `MAILVOID_EVENT_LOG`
  (`<data-dir>/events.jsonl` por defecto). Solo se añade, nunca se modifica.
- **Tabla `events` de Genji** — historial consultable usado por la consola y la API.
- **Anillo en memoria** — los últimos 256 eventos, usado para vistas muy recientes.

Los destinos se invocan de forma síncrona antes de difundir un evento. Los
suscriptores en vivo reciben los eventos con el mejor esfuerzo posible; los
consumidores lentos descartan eventos en lugar de bloquear el servidor. El archivo
JSONL y la base de datos son el registro duradero.

## Transportes en vivo

### WebSocket

`GET /ws/events` se actualiza a WebSocket y envía cada evento como un frame de
texto JSON. La consola usa este transporte.

- requiere una cookie de sesión válida;
- el servidor envía frames de ping cada 25 segundos;
- el plazo de escritura es de 10 segundos;
- sin reproducción: los eventos emitidos antes de la conexión no se reenvían.

### Server-Sent Events

`GET /api/events/stream` transmite:

```text
event: ready
data: {"ok":true}

event: smtp.connected
data: {"id":"ev_...","timestamp":"...","type":"smtp.connected","severity":"info","payload":{...}}

: ping
```

- requiere una cookie de sesión válida;
- comentario de latido cada 20 segundos;
- sin reproducción ni soporte de `Last-Event-ID`.

Usa el endpoint de historial si necesitas eventos ocurridos antes de conectarte.

## Consola

La página **Flujo de eventos**:

- carga los eventos persistidos más recientes (50 por defecto);
- antepone los nuevos eventos por WebSocket;
- muestra un punto en vivo verde mientras está conectada y un punto ámbar mientras
  reconecta;
- admite **Pausar** (almacena en búfer hasta 200 eventos en el navegador) y
  **Reanudar**;
- admite **Limpiar vista** (no elimina los eventos del lado del servidor);
- mantiene como máximo 500 filas en el DOM del navegador.

Los filtros de tipo, severidad y búsqueda en la carga se aplican en el lado del
servidor al cargar la página. Los eventos nuevos en vivo no se filtran en el lado
del cliente; limpia la vista o recarga después de cambiar los filtros.

## Historial y filtros

```bash
# newest first
curl -s -b cookies.txt 'http://localhost:8025/api/events?limit=50'

# by type
curl -s -b cookies.txt \
  'http://localhost:8025/api/events?type=message.dropped'

# by severity
curl -s -b cookies.txt \
  'http://localhost:8025/api/events?severity=error'

# substring search over type, severity, and serialized payload
curl -s -b cookies.txt \
  'http://localhost:8025/api/events?q=spam'
```

El historial de eventos es de solo adición y no tiene compactación automática.
Planifica el crecimiento de disco y base de datos para despliegues de gran
volumen; consulta [Rendimiento](/operations/performance/).

## Convenciones de severidad

| Severidad | Se usa para |
|----------|----------|
| `debug` | Reservado para futuros eventos detallados |
| `info` | Ciclo de vida normal: conexiones, correo almacenado, inicios de sesión, cambios de políticas |
| `warn` | Destinatarios rechazados, correo descartado, mensajes no analizables, inicios de sesión fallidos |
| `error` | Fallos de almacenamiento, fallos del motor de políticas, errores de listener |

Consulta [Referencia de eventos](/reference/events/) para cada tipo y campo de
carga.
