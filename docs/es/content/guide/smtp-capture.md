---
title: Captura SMTP
weight: 10
---

# Captura SMTP

## Listeners

MailVoid abre un listener TCP por cada puerto SMTP configurado. Todos los
listeners comparten el mismo backend, base de datos, motor de políticas y bus de
eventos. Un mensaje recibido en el puerto `2525` es idéntico a uno recibido en
`1025` salvo por su `local_port` registrado.

```text
MAILVOID_SMTP_HOST=0.0.0.0
MAILVOID_SMTP_PORTS=1025,2525,3025
```

El servidor no usa TLS y no anuncia `STARTTLS` ni `AUTH`. Es SMTP plano en los
puertos configurados.

> [!WARNING]
> Las credenciales SMTP enviadas a MailVoid viajan en claro. Acepta correo solo
> desde redes de confianza, o tuneliza SMTP a través de una red privada como una
> VPN o WireGuard.

## Captura del sobre

Para cada destinatario aceptado, MailVoid registra:

| Campo | Origen |
|-------|--------|
| `remote_ip` | Dirección del par TCP |
| `local_port` | Puerto del listener que aceptó el mensaje |
| `sender` | Dirección `MAIL FROM` (vacía para rebotes `<>`) |
| `recipient` | Dirección `RCPT TO` completa, en minúsculas |
| `mailbox` | Parte local antes de `@` |
| `domain` | Parte de dominio después de `@`, en minúsculas |
| `received_at` | Hora del servidor en UTC |
| `size` | Tamaño del mensaje en bruto en bytes |
| `subject` | Cabecera `Subject` decodificada (RFC 2047) |
| `headers` | Mapa de cabeceras analizadas |
| `raw_headers` | Bloque de cabeceras exactamente como se recibió |
| `raw` | Mensaje en bruto completo |
| `body_text` | Cuerpo `text/plain` decodificado |
| `body_html` | Cuerpo `text/html` decodificado |

Las direcciones del sobre se convierten a minúsculas. Los nombres para mostrar en
los comandos del sobre se normalizan a la dirección simple cuando `net/mail` puede
analizarlos.

## Aceptación de dominio

`RCPT TO` se evalúa de inmediato:

- Si la dirección está mal formada: `501 invalid recipient address`.
- Si el dominio no está permitido: `550 relay access denied`, con un evento
  `smtp.rcpt_to` que contiene `accepted:false` y `reason`.
- Si el dominio está permitido: el destinatario se encola para `DATA`.

El remitente de `MAIL FROM` no está restringido por la lista de dominios.

## Múltiples destinatarios

Un mensaje enviado a varios destinatarios en una misma transacción se almacena una
vez por destinatario. Cada copia lleva los mismos bytes en bruto, remitente, asunto
y cuerpos, pero su propio `recipient`, `mailbox` y `domain`. Las políticas se
evalúan por separado para cada destinatario, de modo que una regla dirigida a
`mailbox=billing` no afecta a un mensaje entregado a `mailbox=alerts`.

## Decodificación MIME

El analizador maneja:

- `multipart/*` de forma recursiva;
- codificaciones de transferencia `base64` y `quoted-printable`;
- palabras codificadas RFC 2047 en el asunto;
- conjuntos de caracteres UTF-8, US-ASCII, ISO-8859-1/Latin-1 y Windows-1252;
- alternativas de texto y HTML.

Los adjuntos no se incluyen en `body_text` ni `body_html`, pero siguen disponibles
en el mensaje en bruto. Las partes `message/rfc822` no se analizan de forma
recursiva. Consulta [Análisis MIME](/reference/mime-parsing/) para las reglas
exactas.

## Respuestas SMTP

| Condición | Respuesta |
|-----------|----------|
| Destinatario aceptado, mensaje almacenado | `250 OK` |
| Dominio del destinatario no aceptado | `550 relay access denied` |
| Destinatario mal formado | `501 invalid recipient address` |
| Remitente mal formado | `501 invalid sender address` |
| Sin destinatarios antes de `DATA` | `503 no recipients` |
| Mensaje mayor que el límite | `552 message too large` |
| Fallo de almacenamiento | `451 temporary storage failure` |
| Error de evaluación de políticas | `250 OK` con el mensaje conservado y un evento de advertencia |

`451` indica a un cliente bien comportado que reintente. `550`, `501`, `503` y
`552` son fallos permanentes para esa transacción.

## Mensajes no analizables

`net/mail` puede rechazar bloques de cabeceras mal formados. MailVoid no pierde el
mensaje:

- los bytes en bruto se almacenan;
- el asunto pasa a ser `(unparsable message)`;
- los cuerpos decodificados quedan vacíos;
- se emite un evento `smtp.error` con `stored:true` y el error del analizador.

El mensaje original en bruto sigue disponible en la vista de detalle y en la API.

## Interacción con las políticas

Las políticas se aplican después de leer y analizar el mensaje completo.

- `keep` o ninguna regla coincidente: el mensaje se almacena y se emite un evento
  `message.received`.
- `ttl`: el mensaje se almacena con `action=ttl`, `expires_at` y metadatos de la
  política.
- `drop`: el mensaje no se almacena; se emite un evento `message.dropped`. El
  cliente SMTP sigue recibiendo `250 OK`.

## Sesión de ejemplo

```text
$ swaks --server 127.0.0.1:1025 \
    --from sender@example.org \
    --to inbox@example.test \
    --header "Subject: capture test" \
    --body "hello"

<-  220 mailvoid.local ESMTP
 ->  EHLO localhost
<-  250-mailvoid.local
 ->  MAIL FROM:<sender@example.org>
<-  250 OK
 ->  RCPT TO:<inbox@example.test>
<-  250 OK
 ->  DATA
<-  354 Start mail input
...
<-  250 OK
```

En la traza de auditoría esto produce `smtp.connected`, `smtp.mail_from`,
`smtp.rcpt_to`, `smtp.data` y `message.received` (además de `smtp.closed` cuando
termina la sesión).
