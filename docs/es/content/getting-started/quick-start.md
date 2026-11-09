---
title: Inicio rápido
weight: 20
---

# Inicio rápido

Este recorrido captura un mensaje, inicia sesión y ejercita una política.

## 1. Iniciar MailVoid

```bash
MAILVOID_ADMIN_USER=admin \
MAILVOID_ADMIN_PASSWORD=secret123 \
MAILVOID_SMTP_PORTS=1025,2525 \
MAILVOID_HTTP_ADDR=:8025 \
./bin/mailvoid
```

La configuración predeterminada ya usa los mismos puertos y dirección HTTP; los
valores explícitos hacen que el ejemplo se explique por sí mismo.

## 2. Enviar un mensaje

Usando `swaks`:

```bash
swaks --server 127.0.0.1:1025 \
  --from sender@example.org \
  --to inbox@example.test \
  --header "Subject: Hello from MailVoid" \
  --body "This is the plain-text body."
```

Usando la biblioteca estándar de Python:

```python
import smtplib
from email.mime.text import MIMEText

msg = MIMEText("This is the plain-text body.")
msg["Subject"] = "Hello from MailVoid"
msg["From"] = "sender@example.org"
msg["To"] = "inbox@example.test"

with smtplib.SMTP("127.0.0.1", 1025, timeout=5) as smtp:
    smtp.sendmail("sender@example.org", ["inbox@example.test"], msg.as_string())
```

Envía un mensaje multiparte al segundo puerto para verificar la captura
multipuerto:

```python
from email.mime.multipart import MIMEMultipart
from email.mime.text import MIMEText
import smtplib

msg = MIMEMultipart("alternative")
msg["Subject"] = "Multipart on 2525"
msg["From"] = "sender@example.org"
msg["To"] = "inbox@example.test"
msg.attach(MIMEText("plain text"))
msg.attach(MIMEText("<p>HTML body</p>", "html"))

with smtplib.SMTP("127.0.0.1", 2525, timeout=5) as smtp:
    smtp.sendmail("sender@example.org", ["inbox@example.test"], msg.as_string())
```

## 3. Iniciar sesión

Abre <http://localhost:8025> e inicia sesión con `admin` / `secret123`.

También puedes autenticarte con curl y conservar la cookie de sesión:

```bash
curl -s -c cookies.txt \
  -d 'username=admin&password=secret123' \
  http://localhost:8025/login
```

## 4. Inspeccionar la captura

- **Panel** muestra los totales y los mensajes y eventos más recientes.
- **Mensajes** lista cada mensaje capturado con el puerto receptor, la IP de
  origen, el buzón de destino, el remitente, el asunto y la acción.
- Abre un mensaje para ver los metadatos del sobre, la vista previa HTML en
  sandbox, el cuerpo de texto, las cabeceras y los bytes en bruto.

De forma programática:

```bash
curl -s -b cookies.txt http://localhost:8025/api/messages | jq
curl -s -b cookies.txt http://localhost:8025/api/messages/<id> | jq
```

## 5. Añadir una política de retención

Crea una regla que descarte correo de un dominio de spam conocido:

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
  http://localhost:8025/api/policies | jq
```

Envía un mensaje de `promo@spam.test` a `inbox@example.test`. El cliente SMTP ve
éxito, pero el mensaje se descarta. Confírmalo con la lista de mensajes y el
flujo de eventos:

```bash
curl -s -b cookies.txt \
  'http://localhost:8025/api/events?type=message.dropped' | jq
```

Crea una regla TTL:

```bash
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{
        "name": "expire after 60 seconds",
        "field": "subject",
        "pattern": "temporary",
        "action": "ttl",
        "ttl_seconds": 60,
        "enabled": true
      }' \
  http://localhost:8025/api/policies | jq
```

Envía un mensaje con `Subject: temporary test`. Se almacena con
`"action":"ttl"` y un valor `expires_at`; el barrido lo elimina tras el TTL y
emite `message.deleted` con motivo `ttl expiry`.

## 6. Ver eventos en vivo

Abre **Flujo de eventos** en la consola. La página carga los eventos persistidos
recientes y luego antepone los nuevos por WebSocket. El punto de estado está en
verde mientras el socket está conectado. Usa **Pausar** para congelar la vista y
**Limpiar vista** para eliminar las filas del lado del cliente sin tocar el
registro de auditoría.

Alternativas programáticas:

```bash
# Server-Sent Events
curl -N -b cookies.txt http://localhost:8025/api/events/stream

# JSON history
curl -s -b cookies.txt http://localhost:8025/api/events | jq
```

## 7. Detener

Pulsa `Ctrl-C` o envía `SIGTERM`. MailVoid cierra los listeners SMTP, el servidor
HTTP, los flujos en vivo, el log de eventos y la base de datos en ese orden.
