---
title: Políticas
weight: 30
---

# Políticas

Las políticas deciden qué ocurre con un mensaje a su llegada. Cada regla apunta a
un campo del mensaje con una expresión regular de Go y elige una acción.

## Modelo de evaluación

1. En `DATA`, MailVoid carga todas las políticas **habilitadas** ordenadas por
   `priority` ascendente y, en caso de empate, por hora de creación.
2. Gana la primera regla cuya regex coincida con el campo objetivo.
3. Si ninguna regla coincide, el mensaje se conserva de forma permanente.

No existe acumulación de "todas las reglas coincidentes". El orden importa, y la
consola te permite mover las reglas hacia arriba y hacia abajo.

## Campos objetivo

| Campo | Valor con el que coincide la expresión |
|-------|----------------------------------|
| `sender` | Dirección `MAIL FROM` del sobre |
| `recipient` | Dirección `RCPT TO` completa (`mailbox@domain`) |
| `mailbox` | Solo la parte local |
| `domain` | Solo la parte de dominio |
| `subject` | Asunto decodificado |
| `remote_ip` | Dirección IP del par TCP |
| `local_port` | Puerto de recepción como cadena decimal |
| `any` | Todos los anteriores unidos por espacios |

`any` es útil para reglas amplias, pero puede producir coincidencias sorprendentes
porque los campos se concatenan. Cuando sea posible, prefiere un campo específico.

## Acciones

| Acción | Efecto | ¿Se almacena el mensaje? | Caducidad |
|--------|--------|-----------------|--------|
| `keep` | Conservar de forma permanente | Sí | Ninguna |
| `ttl` | Conservar hasta una fecha límite | Sí | `now + ttl_seconds` |
| `drop` | Descartar en la llegada | No | No aplica |

- `ttl` requiere `ttl_seconds > 0`. Para cualquier otra acción, el `ttl_seconds`
  almacenado se normaliza a `0`.
- `drop` devuelve éxito al cliente SMTP y luego descarta el mensaje. Este es el
  comportamiento intencional de agujero negro; úsalo cuando quieras que los
  remitentes crean que el mensaje fue aceptado.
- Un mensaje TTL conserva el ID y el nombre de la política coincidente para que la
  página de detalle y la API puedan explicar por qué existe.

## Expresiones regulares

MailVoid usa el paquete `regexp` de Go, que implementa RE2:

- coincidencia en tiempo lineal, segura frente a retroceso catastrófico;
- **sin retroreferencias** (`\1`) y **sin lookaround** (`(?=...)`, `(?<=...)`);
- la coincidencia distingue mayúsculas y minúsculas por defecto;
- usa `(?i)` para coincidencias sin distinción de mayúsculas;
- los anclajes `^` y `$` se aplican al valor completo del campo.

Ejemplos:

| Patrón | Coincide con |
|---------|---------|
| `@spam\.example$` | Remitentes que terminan en `@spam.example` |
| `(?i)^invoice` | Asuntos que empiezan por `invoice` en cualquier combinación de mayúsculas |
| `^billing@` | Destinatarios cuyo buzón es `billing` |
| `^192\.0\.2\.` | IP remotas en el rango de documentación |
| `^(1025|2525)$` | Cualquiera de los dos puertos de recepción |
| `.*` | Todo |

Los patrones de más de 2048 caracteres se rechazan. Los patrones no válidos se
rechazan en el límite de la API/formulario y se omiten si de algún modo llegan al
almacenamiento.

## Crear reglas

### Consola

Usa **Políticas** → el formulario de creación. Los campos son:

- **Nombre** — etiqueta legible para humanos; obligatoria.
- **Campo** — uno de los campos objetivo.
- **Patrón** — la expresión regular de Go.
- **Acción** — `keep`, `ttl` o `drop`.
- **Segundos TTL** — obligatorio para `ttl`.
- **Habilitada** — marcada por defecto.

La regla se añade al final del orden de evaluación.

### API

```bash
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{
        "name": "expire notifications after one hour",
        "field": "subject",
        "pattern": "(?i)^notification",
        "action": "ttl",
        "ttl_seconds": 3600,
        "enabled": true
      }' \
  http://localhost:8025/api/policies
```

## Edición y ordenación

- **Editar** abre la regla en el panel del formulario. Guardar conserva su
  posición, hora de creación e ID.
- **↑ / ↓** intercambia la regla con su vecina y renumera las prioridades desde cero.
- **Habilitar / Deshabilitar** alterna la evaluación sin eliminar la regla.
- **Eliminar** la quita de inmediato.

Los equivalentes de la API:

```bash
# full update
curl -s -b cookies.txt -X PUT \
  -H 'Content-Type: application/json' \
  -d '{
        "name": "renamed",
        "field": "sender",
        "pattern": "@spam\\.test$",
        "action": "drop",
        "enabled": true
      }' \
  http://localhost:8025/api/policies/<policy-id>

# reorder: every policy id exactly once
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"ids":["<second-id>","<first-id>","<third-id>"]}' \
  http://localhost:8025/api/policies/reorder
```

`PUT` es un reemplazo completo de los campos de texto de la regla: envía cada
cadena y `ttl_seconds` que quieras conservar. Las cadenas y `ttl_seconds` omitidos
se convierten en valores cero, lo que normalmente falla la validación. Los campos
`enabled` y `priority` omitidos conservan los valores existentes.

## Probar expresiones

El panel **Probador de regex** evalúa un campo, un patrón y un valor de ejemplo sin
guardar una regla:

```bash
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"field":"subject","pattern":"^invoice","value":"Invoice 42"}' \
  http://localhost:8025/api/policies/test
```

```json
{"field":"subject","matched":true,"pattern":"^invoice","value":"Invoice 42"}
```

El probador es solo una comprobación de coincidencia. No muestra qué acción
ganaría frente a la lista completa y ordenada de políticas.

## Eliminación por TTL

El TTL se aplica mediante una goroutine sweeper:

- se ejecuta una vez al arrancar y luego cada `MAILVOID_SWEEP_INTERVAL`;
- llama a `DeleteExpired(now)` en el repositorio;
- elimina los mensajes cuyo `expires_at` es distinto de cero y `<= now`;
- emite `message.deleted` por cada mensaje con `reason: "ttl expiry"`.

Un mensaje puede permanecer visible hasta un intervalo de barrido después de que
venza su temporizador.

## Ideas de recetas

| Objetivo | Campo | Patrón | Acción |
|------|-------|---------|--------|
| Descartar un remitente de spam conocido | `sender` | `@spam\.test$` | `drop` |
| Conservar facturas para siempre | `subject` | `(?i)invoice` | `keep` |
| Caducar todas las notificaciones en un día | `subject` | `(?i)^notification` | `ttl` 86400 |
| Caducar todo en el puerto de prueba | `local_port` | `^3025$` | `ttl` 3600 |
| Descartar correo de una subred de laboratorio | `remote_ip` | `^10\.20\.` | `drop` |
| Caducar un buzón específico | `mailbox` | `^qa-automation$` | `ttl` 7200 |

Coloca las reglas `drop` específicas antes que las reglas `keep` amplias. Como gana
la primera coincidencia, una regla `keep` comodín al principio hace que todas las
reglas posteriores sean inalcanzables.
