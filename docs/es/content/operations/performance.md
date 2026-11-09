---
title: Rendimiento
weight: 70
---

# Rendimiento

MailVoid está diseñado para desarrollo, QA y volúmenes de captura de bajos a
moderados. No es una pasarela de correo de alto rendimiento. Comprende los
centros de coste antes de dimensionar un despliegue.

## Coste por mensaje

Para una transacción SMTP con `N` destinatarios, MailVoid realiza:

1. Un análisis MIME.
2. Una carga de la lista de políticas y una evaluación de expresiones regulares
   por destinatario (las expresiones regulares compiladas se almacenan en caché).
3. Una escritura en la base de datos por destinatario (el mensaje en bruto se
   duplica por destinatario).
4. Varias emisiones de eventos: conexión, MAIL FROM, RCPT TO por destinatario,
   DATA, coincidencia de política y mensaje recibido/descartado.

Cada evento es:

- anexado a `events.jsonl`;
- insertado en la tabla `events` de Genji;
- añadido al búfer circular en memoria;
- difundido a los suscriptores en vivo (sin bloqueo).

Los sumideros de archivo y de base de datos son síncronos. Un disco lento ralentiza
directamente el manejo de `DATA` de SMTP. Los clientes lentos en vivo no ralentizan
el servidor porque su canal de suscripción no bloquea; simplemente se pierden
eventos.

## Puntos de serialización

| Componente | Comportamiento |
|-----------|----------|
| Escritor de Genji | Un mutex exclusivo para todas las escrituras (mensajes, eventos, políticas) |
| Lector de Genji | RWMutex compartido |
| Sumideros de eventos | Se invocan en la goroutine del emisor |
| Caché de expresiones regulares | `sync.Map`, compiladas una vez por patrón |
| Barrido de TTL | Una goroutine; busca mensajes caducados |

Cada conexión SMTP se ejecuta en su propia goroutine, pero las escrituras se
serializan en el repositorio. El rendimiento está limitado por la ruta de
escritura de la base de datos, no por el planificador de Go.

## Consultas costosas

Genji v0.15 no expone índices secundarios para estos patrones de acceso, por lo
que las siguientes operaciones recorren la tabla correspondiente:

- filtros `?q=` que usan `LIKE '%...%'` sobre `search_text`;
- `DeleteExpired`, que recorre los mensajes buscando un `expires_at` distinto de
  cero dentro del rango;
- `COUNT(*)` para los totales del panel de control y la API;
- recuento de eventos para el panel de control.

A pequeña escala esto es invisible. Con cientos de miles de mensajes, espera que
la búsqueda, los recuentos y el barrido tarden un tiempo medible.

## Disco y sistema de archivos

- Coloca el directorio de datos en almacenamiento SSD/NVMe local.
- Evita NFS, SMB u otros sistemas de archivos de red para el directorio de Genji;
  Pebble espera un bloqueo local fiable y una semántica de fsync correcta.
- Deja margen para la compactación de Pebble y los registros de escritura
  anticipada.
- El registro de eventos JSONL crece sin límite. Archívalo de forma programada.
- No ejecutes varias instancias contra un mismo directorio de datos.

## Memoria

La memoria está dominada por:

- el runtime de Go y Fiber;
- la caché de bloques y las memtables de Pebble;
- los mensajes en bruto en curso (hasta `MAILVOID_MAX_MESSAGE_BYTES` cada uno);
- el búfer circular de 256 eventos;
- los búferes de los suscriptores en vivo.

Un mensaje máximo de 25 MiB no significa que cada mensaje use 25 MiB; es un
límite superior. Los mensajes grandes concurrentes pueden multiplicar el uso de
memoria.

## Búsqueda de texto completo

- Bleve indexa el texto de forma síncrona después de almacenar un mensaje. Por lo
  tanto, cada mensaje aceptado añade trabajo de escritura de índice a la ruta de
  captura SMTP.
- El directorio del índice crece con el contenido de los mensajes y el margen de
  compactación.
- El arranque puede reconstruir el índice cuando el número de documentos difiere
  del almacén; las instalaciones grandes pueden hacer que el primer arranque tras
  perder el índice sea lento.
- Si la indexación falla, el mensaje se almacena de todos modos y se emite un
  evento `search.error`; la búsqueda puede quedar desactualizada hasta una
  reconstrucción.
- `GET /api/v1/search` y `search_messages` de MCP evitan el recorrido completo de
  la tabla de mensajes que usa el respaldo por subcadenas para las consultas de
  cuerpo.

## Consultas de agente y WebDAV

- `GET /api/v1/mailboxes` y la herramienta `list_mailboxes` de MCP agregan
  mensajes recorriendo el almacén de mensajes; el coste crece con el total de
  correo capturado.
- `GET /api/v1/mailboxes/{address}` usa recuentos y una búsqueda de una sola fila.
- `wait_for_email` y `/wait` sondean el almacén cada 500 ms durante un máximo de
  60 segundos. Muchas esperas concurrentes añaden carga de lectura.
- `PROPFIND` de WebDAV sobre `/dav/` agrega todos los buzones; listar un buzón
  pagina por todos sus mensajes. Los árboles grandes hacen más lentos los
  listados de directorios.
- Las descargas `.eml` transmiten los bytes en bruto almacenados y son baratas en
  comparación con la agregación.

## HTTP y recursos

- El paquete DaisyUI incrustado ocupa unos 2,9 MiB antes de la compresión HTTP.
  Se almacena en caché durante una hora, pero supone un coste notable en la
  primera carga.
- Las conexiones Server-Sent Events y WebSocket consumen una goroutine y búferes
  por cliente.
- Las tarjetas de recuento del panel de control ejecutan consultas agregadas en
  cada carga de página.

## Planificación de capacidad

No existe una cifra universal de mensajes por segundo; depende del tamaño del
mensaje, el número de destinatarios, la latencia del disco, el volumen de eventos
y el número de políticas. Haz una prueba de rendimiento con tu propia carga:

```bash
# Send 100 small messages and time it.
time for i in $(seq 1 100); do
  swaks --server 127.0.0.1:1025 \
    --from sender@example.org \
    --to inbox@example.test \
    --header "Subject: load $i" \
    --body "small body" >/dev/null
done

curl -s -b cookies.txt http://127.0.0.1:8025/api/stats | jq
```

Vigila:

- `iostat` / `nvme-cli` para la latencia del disco;
- el espacio libre;
- el RSS del proceso;
- el tamaño del directorio de Genji;
- el crecimiento del recuento de eventos.

## Opciones de escalado

| Necesidad | Enfoque |
|------|----------|
| Más puertos/dominios en un host | Añade puertos/dominios a la misma instancia |
| Aislamiento entre proyectos | Instancias de MailVoid separadas con directorios de datos y puertos distintos |
| Más rendimiento que una instancia | Ejecuta varias instancias con directorios de datos independientes y enruta los clientes |
| Historial de eventos a largo plazo | Archiva el JSONL y exporta/elimina los eventos de la base de datos externamente |
| Menos almacenamiento | Usa políticas TTL de forma agresiva; elimina mensajes antiguos vía API |

No puedes compartir un directorio de Genji entre instancias. El escalado
horizontal es particionado, no clustering.

## Optimizaciones futuras

No implementadas en esta versión:

- sumideros de eventos asíncronos o por lotes;
- retención/compactación de eventos;
- índices secundarios para búsqueda y caducidad;
- análisis MIME en streaming para mensajes muy grandes;
- compresión de recursos estáticos incrustados;
- límites de tasa SMTP por cliente;
- opciones configurables de caché y escritura de Pebble.

La costura del repositorio y el bus de eventos están estructurados para que
estas mejoras puedan añadirse sin cambiar las APIs SMTP o web.
