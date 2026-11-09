---
title: Generador de correo Lorem
weight: 55
---

# Generador de correo Lorem

`lorem-mail-generator` se conecta a un listener SMTP de MailVoid y envía mensajes
Lorem Ipsum aleatorios con recuentos de palabras configurables. Es útil para
ejercitar el panel, la búsqueda, la paginación, las políticas de retención y el
manejo de carga SMTP.

## Compilación

```bash
make lorem
```

El binario se escribe en `bin/lorem-mail-generator`.

## Uso

```bash
./bin/lorem-mail-generator -addr 127.0.0.1:1025 -count 100
```

Cada mensaje recibe un asunto aleatorio, un `Message-ID` generado y dos
cabeceras de prueba:

- `X-Mailvoid-Lorem-Index`: índice del mensaje, comenzando en cero.
- `X-Mailvoid-Lorem-Words`: cantidad de palabras solicitada para el cuerpo.

## Flags

| Flag | Valor predeterminado | Descripción |
|------|----------------------|-------------|
| `-addr` | `127.0.0.1:1025` | Dirección del servidor SMTP |
| `-from` | `lorem@example.test` | Remitente del sobre |
| `-to` | `inbox@example.test` | Destinatarios separados por comas |
| `-count` | `10` | Cantidad de mensajes a enviar |
| `-min-words` | `10` | Mínimo de palabras del cuerpo |
| `-max-words` | `120` | Máximo de palabras del cuerpo |
| `-concurrency` | `1` | Conexiones SMTP en paralelo |
| `-delay` | `0` | Retardo entre mensajes por worker |
| `-timeout` | `10s` | Tiempo de espera por conexión |
| `-seed` | `0` | Semilla aleatoria; `0` usa la hora actual |
| `-html` | `false` | Envía cuerpos HTML en lugar de texto plano |
| `-subject-prefix` | `Lorem` | Prefijo del asunto |
| `-fail-fast` | `false` | Detiene todo al primer error de envío |
| `-quiet` | `false` | Suprime la salida de progreso |
| `-random-mailboxes` | `0` | Genera N mailboxes aleatorios; anula `-to` |
| `-mailbox-prefix` | `lorem` | Prefijo de la parte local para mailboxes aleatorios |
| `-domains` | `example.test` | Dominios separados por comas para mailboxes aleatorios |

## Ejemplos

### Prueba de volumen

```bash
./bin/lorem-mail-generator \
  -addr 127.0.0.1:1025 \
  -count 500 \
  -min-words 20 \
  -max-words 200 \
  -concurrency 8
```

### Ejecución reproducible

```bash
./bin/lorem-mail-generator -seed 42 -count 25
```

### Cuerpos HTML

```bash
./bin/lorem-mail-generator -html -min-words 50 -max-words 500
```

### Varios destinatarios fijos

```bash
./bin/lorem-mail-generator -to 'inbox@example.test,alerts@example.test' -count 20
```

Cada mensaje se entrega a todas las direcciones de `-to`.

### Mailboxes aleatorios

```bash
./bin/lorem-mail-generator \
  -count 500 \
  -random-mailboxes 40 \
  -domains 'example.test,test.local' \
  -mailbox-prefix load
```

Esto construye un pool de 40 direcciones únicas como
`load-dolor-4821@test.local` y asigna un mailbox aleatorio por mensaje.
Cuando `-random-mailboxes` es mayor que cero, se ignora `-to`.

## Comportamiento

- Cada mensaje usa una conexión SMTP nueva, por lo que `-concurrency` controla
  las conexiones en paralelo.
- Con `-random-mailboxes`, cada mensaje tiene exactamente un destinatario elegido
  del pool generado, lo que hace más realistas las listas de mailboxes y la
  paginación.
- MailVoid no implementa autenticación SMTP ni STARTTLS, así que esta
  herramienta no intenta ninguna de las dos.
- El comando imprime un resumen con la semilla, los envíos correctos y fallidos,
  el tiempo transcurrido y los mensajes por segundo.
- El código de salida es `1` cuando falla algún mensaje, o `2` para flags
  inválidos.
