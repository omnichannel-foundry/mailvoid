---
title: Mensajes
weight: 20
---

# Mensajes

## Listado

La página **Mensajes** lista el correo capturado, del más reciente al más antiguo.
Cada fila muestra:

- asunto y remitente del sobre;
- buzón y dominio de destino;
- puerto de recepción;
- IP de origen;
- acción de la política (`keep`, `ttl` o `drop` — los mensajes descartados nunca
  aparecen);
- marca de tiempo de recepción.

Los filtros usan HTMX para actualizaciones en el mismo lugar:

| Filtro | Comportamiento |
|--------|----------|
| Buscar | Coincidencia de subcadena sin distinción de mayúsculas sobre remitente, destinatario, buzón, dominio, asunto e IP remota. Para búsqueda clasificada en el cuerpo usa [Búsqueda de texto completo](/reference/search/). |
| Dominio | Coincidencia exacta de dominio sin distinción de mayúsculas |
| Buzón | Coincidencia exacta de la parte local sin distinción de mayúsculas |
| Puerto | Puerto de recepción exacto |
| Acción | `keep` o `ttl` (los mensajes descartados no se almacenan) |

La paginación es de 50 filas por página por defecto y tiene un máximo de 200. La
API JSON acepta los mismos filtros.

## Vista de detalle

Abre un mensaje para ver:

### Tarjeta del sobre

Buzón de destino, IP de origen, puerto de destino, hora de recepción, tamaño, ID
de mensaje, política coincidente y caducidad.

### Vista previa HTML

Si el mensaje tiene un cuerpo HTML, se renderiza dentro de un `<iframe>` aislado:

```html
<iframe sandbox referrerpolicy="no-referrer" srcdoc="..."></iframe>
```

El sandbox no tiene `allow-scripts` ni `allow-same-origin`, por lo que los scripts
del correo no se ejecutan en el origen de MailVoid. Los recursos remotos aún pueden
cargarse dentro del frame aislado; usa aislamiento de red si eso es importante.

### Cuerpo de texto plano

La parte `text/plain` decodificada, con un botón de copia.

### Cabeceras

Una tabla de cabeceras analizadas más el bloque de cabeceras en bruto, con un
botón de copia.

### Mensaje en bruto

Los bytes exactos que llegaron, con un botón de copia.

## Cuenta atrás de caducidad

Los mensajes con una política TTL muestran una cuenta atrás en vivo. El servidor
renderiza una marca de tiempo RFC 3339 en `data-expires`; el cliente actualiza el
texto cada segundo. Cuando el temporizador llega a cero, el sweeper elimina el
mensaje en el siguiente tick de barrido, y la página puede seguir mostrando el
mensaje hasta que se recargue.

El sweeper se ejecuta cada `MAILVOID_SWEEP_INTERVAL` (`30s` por defecto). Un
mensaje no desaparece en el segundo exacto en que vence su TTL; desaparece en el
siguiente barrido tras la caducidad.

## Eliminación

Usa el botón **Eliminar** en la página de detalle o la API:

```bash
curl -s -b cookies.txt -X DELETE \
  http://localhost:8025/api/messages/<message-id>
```

La eliminación es inmediata y permanente. Emite `message.deleted` con:

- `reason`: `manual deletion` o `api deletion`;
- `username`: la cuenta autenticada;
- metadatos del mensaje para la traza de auditoría.

Las eliminaciones por TTL usan `reason: "ttl expiry"` y ningún nombre de usuario.

## Descarga de .eml

Usa **Descargar .eml** en la página de detalle, o la API versionada:

```bash
curl -s -b cookies.txt -o message.eml \
  http://localhost:8025/api/v1/messages/<message-id>/eml
```

Los clientes WebDAV pueden obtener los mismos bytes desde
`/dav/<mailbox>@<domain>/<message-id>.eml`. Consulta [WebDAV](/operations/webdav/).

## Acceso a la API

```bash
# list
curl -s -b cookies.txt \
  'http://localhost:8025/api/messages?q=invoice&port=1025&limit=20'

# detail
curl -s -b cookies.txt \
  http://localhost:8025/api/messages/<message-id>
```

La lista devuelve solo metadatos. El endpoint de detalle devuelve el registro
completo, incluidas cabeceras, cabeceras en bruto, mensaje en bruto y cuerpos
decodificados.

## Formato de almacenamiento

Cada mensaje es una fila en la tabla `messages` de Genji y un objeto JSON en las
respuestas de la API. Las cabeceras se almacenan como un objeto JSON de arrays de
cadenas. Las horas se almacenan como milisegundos Unix y se renderizan como RFC
3339 en JSON. Consulta [Modelo de datos](/reference/data-model/) para el esquema
completo.
