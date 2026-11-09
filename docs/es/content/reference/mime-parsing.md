---
title: Análisis MIME
weight: 60
---

# Análisis MIME

MailVoid usa la biblioteca estándar para las cabeceras RFC 5322 y un pequeño
recorredor MIME recursivo en `internal/mailparse`. El objetivo es la inspección
práctica, no una implementación MIME completa.

## Cabeceras y asunto

`net/mail.ReadMessage` analiza el bloque de cabeceras. Los nombres de las
cabeceras se canonizan y se conservan como `map[string][]string`; el bloque exacto
se guarda en `raw_headers`. Solo se decodifica el asunto:

- Se decodifican las palabras codificadas RFC 2047.
- UTF-8 y US-ASCII se dejan pasar tal cual.
- ISO-8859-1/Latin-1 y Windows-1252 se convierten con `x/text/charmap`.
- Los juegos de caracteres desconocidos se dejan con sus bytes originales.
- Si la decodificación falla, se usa la cadena de asunto original.

`raw_headers` es el bloque de cabeceras exacto, dividido en la primera línea en
blanco.

## Extracción del cuerpo

El recorredor maneja `multipart/*` de forma recursiva:

1. Decodifica el `Content-Transfer-Encoding` de la parte.
2. Si el tipo de medio es `multipart/*`, lo analiza con su límite y recurre.
3. Si es `text/plain` y aún no se ha capturado un cuerpo de texto, decodifica su
   juego de caracteres y lo guarda como `body_text`.
4. Si es `text/html` y aún no se ha capturado un cuerpo HTML, decodifica su juego
   de caracteres y lo guarda como `body_html`.
5. Ignora los demás tipos de medio.

Ganan la primera parte de texto y la primera parte HTML. Las alternativas
posteriores se ignoran para la visualización del cuerpo, pero permanecen en el
mensaje en bruto.

### Codificaciones de transferencia

| Codificación | Tratamiento |
|--------------|-------------|
| `base64` | Se eliminan los espacios en blanco y se decodifica base64 estándar |
| `quoted-printable` | Se decodifica con `mime/quotedprintable` |
| `7bit`, `8bit`, `binary`, ausente | Se dejan pasar tal cual |

### Juegos de caracteres

| Juego de caracteres | Tratamiento |
|---------------------|-------------|
| ausente, `utf-8`, `us-ascii`, `ascii` | Cadena byte a byte |
| `iso-8859-1`, `latin1`, `latin-1` | `charmap.ISO8859_1` |
| `windows-1252`, `cp1252` | `charmap.Windows1252` |
| cualquier otro | Cadena byte a byte |

No se inspeccionan las declaraciones `<meta charset>` del HTML; solo se usa el
parámetro MIME.

## Adjuntos

Las partes con un `Content-Disposition` que comienza por `attachment` se omiten
por completo, incluidos los adjuntos de texto. El mensaje en bruto todavía los
contiene.

## Qué no está implementado

- El análisis recursivo de partes `message/rfc822`.
- La decodificación de cabeceras que no son del cuerpo, como los nombres
  mostrados en `From` o las listas de `To`.
- La detección del juego de caracteres a partir de los metadatos HTML.
- Las continuaciones de parámetros MIME más allá de lo que maneja
  `mime.ParseMediaType`.
- El saneamiento de HTML. La consola aísla el HTML en un iframe en sandbox; la API
  devuelve el HTML sin cambios.

## Gestión de fallos

Si falla el análisis de las cabeceras:

- el mensaje se almacena igualmente;
- `subject` pasa a ser `(unparsable message)`;
- `body_text` y `body_html` quedan vacíos;
- se emite un evento `smtp.error` con `stored:true`.

Si una parte individual del cuerpo no se puede decodificar, MailVoid recurre a los
bytes en bruto de la parte y continúa con las partes restantes. El mensaje en
bruto siempre se conserva para los mensajes retenidos.

## Implicaciones de seguridad

- El HTML nunca se inserta directamente en el origen de MailVoid. Se renderiza en
  un iframe con `sandbox` y sin `allow-scripts` ni `allow-same-origin`.
- El texto plano y los mensajes en bruto se escapan como HTML mediante
  `html/template`.
- Las imágenes y estilos remotos dentro de un correo HTML aún pueden cargarse
  dentro del marco en sandbox. Si eso es un problema, aísla MailVoid de la red o
  inspecciona el mensaje en bruto en lugar de abrir la vista previa HTML.
