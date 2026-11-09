---
title: Solución de problemas
weight: 60
---

# Solución de problemas

## El servicio no arranca

### `listen tcp ...: bind: address already in use`

Otro proceso ocupa un puerto SMTP o el puerto HTTP.

```bash
ss -ltnp | grep -E '1025|2525|8025'
```

Cambia `MAILVOID_SMTP_PORTS` / `MAILVOID_HTTP_ADDR` o detén el otro proceso.

### `bind: permission denied` en el puerto 25 o 587

Los puertos por debajo de 1024 requieren privilegios. Usa los valores
predeterminados sin privilegios, ejecuta con `CAP_NET_BIND_SERVICE`, o redirige
desde un puerto privilegiado con reglas de cortafuegos.

### `open database: ... resource temporarily unavailable`

Otra instancia de MailVoid mantiene el archivo `LOCK` de Genji en el directorio de
datos. Detén la otra instancia o elige un `MAILVOID_DB_PATH` diferente.

### `configuration: ...` al arrancar

Lee el nombre de la variable en el error. Las duraciones, booleanos, recuentos de
bytes y rangos de puertos no válidos fallan antes de que se abra cualquier
listener. Consulta [Configuración](/getting-started/configuration/).

## No se puede iniciar sesión

### Contraseña desconocida

Si no se configuró `MAILVOID_ADMIN_PASSWORD` en la primera ejecución, la
contraseña generada se imprimió una sola vez en el registro de arranque. Revisa el
journal o stdout.

Si se ha perdido, reinicia una vez con una contraseña nueva y el restablecimiento
habilitado:

```bash
MAILVOID_ADMIN_PASSWORD='new-password' \
MAILVOID_ADMIN_RESET_PASSWORD=true \
./mailvoid
```

### El inicio de sesión funciona pero la siguiente solicitud no está autenticada

- ¿Detrás de HTTPS con `MAILVOID_COOKIE_SECURE=false`? La cookie se sigue
  enviando; el flag secure solo restringe HTTP.
- ¿Detrás de HTTPS con `MAILVOID_COOKIE_SECURE=true` pero navegando por HTTP? El
  navegador no enviará la cookie. Usa HTTPS.
- Un proxy inverso que elimina `Set-Cookie` o reescribe la ruta rompe las
  sesiones. Asegúrate de que las cookies pasan sin cambios.
- El almacén de sesiones está en memoria. Reiniciar MailVoid cierra la sesión de
  todos.

### 401 repetido desde la API

Usa un tarro de cookies. La API no acepta tokens bearer:

```bash
curl -s -c cookies.txt -d 'username=admin&password=...' \
  http://127.0.0.1:8025/login
curl -s -b cookies.txt http://127.0.0.1:8025/api/messages
```

## Los clientes SMTP fallan

### `550 relay access denied`

El dominio del destinatario no está en `MAILVOID_DOMAINS`. Inspecciona la carga del
evento `smtp.rcpt_to` para ver `accepted:false` y `reason`. Añade el dominio exacto
o un patrón `*.zone`, o usa `*` solo en un entorno de confianza.

### `552 message too large`

El mensaje en bruto supera `MAILVOID_MAX_MESSAGE_BYTES` (25 MiB por defecto).
Auméntalo solo después de considerar el impacto en disco y memoria.

### `451 temporary storage failure`

La escritura en la base de datos falló. Comprueba el espacio en disco, los
permisos del directorio de datos, el registro del proceso y el evento
`message.error`. El cliente puede reintentar.

### Mensaje aceptado pero no almacenado

Comprueba si hay una política de descarte:

```bash
curl -s -b cookies.txt \
  'http://127.0.0.1:8025/api/events?type=message.dropped' | jq
```

El evento `policy.matched` inmediatamente anterior al descarte muestra qué regla se
activó.

### El destinatario se acepta pero el cuerpo está vacío

Abre el mensaje en bruto. Si es un mensaje MIME mal formado, MailVoid lo almacena
con el asunto `(unparsable message)`. Los adjuntos se excluyen intencionadamente
de los cuerpos decodificados. El correo solo HTML tiene un cuerpo de texto vacío.

## Las políticas no se comportan como se espera

- **Gana la primera coincidencia.** Una regla `keep` amplia con `.*` en prioridad
  0 hace inalcanzables todas las reglas posteriores. Reordénala o elimínala.
- **La expresión regular distingue mayúsculas de minúsculas.** Usa `(?i)`.
- **RE2 no tiene lookaround ni retrorreferencias.** Reescribe el patrón.
- **El campo `any` concatena valores.** Una regla puede coincidir a través de los
  límites de campo; prueba con el campo específico en su lugar.
- **El TTL no se ejecuta al instante.** El barrido se ejecuta cada
  `MAILVOID_SWEEP_INTERVAL` (30s por defecto) y elimina los mensajes en el
  siguiente ciclo tras la caducidad.
- **Expresión regular no válida almacenada manualmente.** Las reglas guardadas a
  través de la API/UI se validan. Un patrón mal formado insertado directamente en
  la base de datos es omitido por el motor y nunca coincide.

Comprueba la decisión real:

```bash
curl -s -b cookies.txt \
  'http://127.0.0.1:8025/api/events?type=policy.matched' | jq
```

## Los eventos en vivo no se actualizan

### WebSocket bloqueado por un proxy

El proxy inverso debe pasar las cabeceras `Upgrade` y `Connection` en
`/ws/events`. Consulta [Proxy inverso](/operations/reverse-proxy/). El punto de
estado se vuelve ámbar y el navegador reintenta con retroceso exponencial.

### SSE se atasca detrás de un proxy

Desactiva el almacenamiento en búfer y la compresión del proxy para
`/api/events/stream`. Nginx: `proxy_buffering off; gzip off;`. Caddy:
`flush_interval -1`.

### No hay eventos en absoluto

`/ws/events` y `/api/events/stream` requieren una cookie de sesión. La consola
también necesita JavaScript habilitado para el socket en vivo; la página inicial
sigue mostrando los eventos persistidos.

## El acceso a la API, MCP o WebDAV falla

### `401` desde `/api/v1` o `/mcp`

- Falta el token bearer o no coincide con `MAILVOID_API_TOKEN`.
- No hay ningún token configurado y la solicitud no tiene cookie de sesión.
- El token se rotó pero el proceso no se reinició.
- Envía `Authorization: Bearer <token>` exactamente; la autenticación Basic no se
  acepta aquí.

### `401` desde `/dav/`

- `MAILVOID_WEBDAV_PASSWORD` no está establecido, así que WebDAV está deshabilitado
  y devuelve `404`.
- El nombre de usuario/contraseña de autenticación Basic es incorrecto.
- El cliente envía las credenciales en una redirección pero no en la solicitud
  original; comprueba la configuración del cliente.

### `405` desde `/dav/`

Solo se permiten `OPTIONS`, `PROPFIND`, `GET` y `HEAD`. El árbol WebDAV es de solo
lectura por diseño.

### La descarga `.eml` devuelve `404`

El ID de mensaje no existe, o la ruta WebDAV usa el directorio de buzón
incorrecto. El nombre de archivo es `<message-id>.eml`.

## La búsqueda devuelve el respaldo o nada

### `backend: "fallback"`

El índice de Bleve está deshabilitado o no disponible. Comprueba:

- `MAILVOID_SEARCH_ENABLED` y `MAILVOID_SEARCH_INDEX`;
- el registro de arranque del proceso para `open search index` / `prepare search index`;
- el historial de eventos para `search.error`.

Si el índice está corrupto, detén MailVoid, elimina el directorio `search.bleve` y
reinicia; el arranque lo reconstruye desde Genji. Si persisten errores de disco o
de permisos, resuélvelos primero.

### La búsqueda no encuentra términos del cuerpo

Usa `GET /api/v1/search` o `search_messages` de MCP; el heredado
`GET /api/v1/messages?q=` es una búsqueda de subcadenas en metadatos. El MIME en
bruto, las cargas base64 y el contenido de los adjuntos no se indexan.

## Fallos de credenciales gestionadas

### `401` con una clave de API gestionada

La clave puede estar eliminada, caducada o mal escrita. Lista las claves con una
sesión de administrador y `GET /api/v1/admin/api-keys`. Crea un reemplazo y elimina
la clave antigua.

### `403` en `/api/v1/admin`

Las claves de API gestionadas no pueden gestionar credenciales. Usa una sesión de
administrador o el `MAILVOID_API_TOKEN` de arranque.

### `/dav/` devuelve `404` tras eliminar usuarios

Cuando no queda ninguna contraseña WebDAV de arranque ni usuarios gestionados, la
superficie WebDAV está deshabilitada. Crea un usuario gestionado a través de la API
de administración para habilitarla.

## La búsqueda no devuelve nada

- La búsqueda de mensajes cubre remitente, destinatario, buzón, dominio, asunto e
  IP de origen. No busca en cuerpos, cabeceras ni el mensaje en bruto.
- La búsqueda de eventos cubre tipo, gravedad y carga serializada.
- La búsqueda no distingue mayúsculas de minúsculas, pero `%` y `_` conservan el
  significado de comodín de `LIKE` de SQL dentro de la consulta.

## El uso de disco crece

Fuentes esperadas:

- mensajes en bruto más cuerpos decodificados;
- una copia del mensaje por cada destinatario del sobre;
- una tabla `events` de solo anexado;
- el registro JSONL de solo anexado;
- los registros de escritura anticipada y el margen de compactación de Pebble.

Mitigaciones:

- añade políticas TTL;
- archiva o trunca `events.jsonl` mientras está detenido;
- exporta y elimina mensajes antiguos vía la API;
- usa un volumen más grande;
- planifica un proceso externo de retención de eventos.

## Las marcas de tiempo parecen incorrectas

El almacenamiento y el JSON están en UTC. La consola muestra UTC. El cliente de
eventos en vivo formatea las horas en la zona horaria del navegador. Si la hora
del host es incorrecta, corrige NTP; la base de datos no tiene un reloj
independiente.

## Fallos del ejecutor de integración

`mailvoid-it` imprime el paso que falla y sus registros. Causas comunes:

- `server binary not found` — ejecuta `make build` primero.
- `server did not become ready` — inspecciona el registro del servidor impreso con
  el error; comprueba conflictos de puertos o una configuración rota.
- `401` — el inicio de sesión de administrador de la prueba falló, o una
  credencial creada ya fue revocada.
- Discrepancia de `backend` en los pasos de búsqueda — el servidor lanzado debe
  tener `MAILVOID_SEARCH_ENABLED=true` (el ejecutor lo establece).
- `status = 404, want 401` para WebDAV — eliminar el último usuario WebDAV
  gestionado deshabilita `/dav/` cuando no existe contraseña de arranque; el
  estado esperado es `404`.

Ejecuta un único archivo con `-test name -batch -verbose`.

## Dónde buscar

| Síntoma | Primer lugar |
|---------|-------------|
| El proceso sale o falla al vincular | `journalctl -u mailvoid`, stdout |
| Rechazo SMTP | Flujo de eventos filtrado por `smtp.rcpt_to` |
| Decisión de política | Flujo de eventos filtrado por `policy.matched` / `message.dropped` |
| Fallo de almacenamiento | Flujo de eventos filtrado por `message.error` / `smtp.error` |
| Problema de inicio de sesión | Flujo de eventos filtrado por `auth.login.failed` |
| Estado de los datos | `GET /api/stats` y el directorio de datos |
