---
title: Preguntas frecuentes
weight: 80
---

# Preguntas frecuentes

## ¿Qué es MailVoid?

Un servidor de captura SMTP de binario único con consola administrativa. Almacena
el correo entrante en una base de datos embebida y lo expone mediante una interfaz
web y una API JSON.

## ¿Es un servidor de correo?

No. Acepta transacciones SMTP, almacena o descarta los mensajes y nunca los
retransmite ni los entrega. No hay IMAP, POP3 ni ningún protocolo de buzón.

## ¿Admite TLS o autenticación SMTP?

No. No hay STARTTLS, TLS implícito ni SMTP AUTH. Usa una red privada o un túnel
para el salto SMTP.

## ¿Puedo usar el puerto 25 o el 587?

Sí, si el proceso tiene `CAP_NET_BIND_SERVICE` o privilegios suficientes. Los
valores predeterminados `1025` y `2525` no requieren privilegios.

## ¿Puede recibir correo para cualquier dominio?

Sí. `MAILVOID_DOMAINS=*` (el valor predeterminado) acepta todos los dominios.
Restríngelo en entornos compartidos. Se admiten dominios exactos y comodines
`*.zone`.

## ¿Cómo envío correo de prueba?

```bash
swaks --server 127.0.0.1:1025 \
  --from sender@example.org \
  --to inbox@example.test \
  --header "Subject: test" \
  --body "hello"
```

O usa `smtplib` de Python. Consulta el [Inicio rápido](/getting-started/quick-start/).

## ¿Dónde se almacenan los datos?

De forma predeterminada:

- `./data/mailvoid.db/` — directorio de la base de datos Genji;
- `./data/events.jsonl` — registro JSON de solo adición.

Cámbialos con `MAILVOID_DATA_DIR`, `MAILVOID_DB_PATH` y
`MAILVOID_EVENT_LOG`.

## ¿Cómo restablezco la contraseña del administrador?

```bash
MAILVOID_ADMIN_PASSWORD='new-password' \
MAILVOID_ADMIN_RESET_PASSWORD=true \
./mailvoid
```

Si omites la contraseña con el restablecimiento habilitado, se genera una nueva
contraseña aleatoria y se imprime una sola vez.

## ¿Por qué desapareció mi mensaje?

Dos causas probables:

1. Coincidió una política `drop` y el mensaje nunca se almacenó.
2. Coincidió una política `ttl` y el barrido lo eliminó tras su caducidad.

Filtra el flujo de eventos por `message.dropped`, `policy.matched` o
`message.deleted`.

## ¿Por qué el flujo de eventos en vivo no se actualiza?

- La ruta WebSocket o SSE requiere una cookie de sesión válida.
- Un proxy inverso puede estar almacenando en búfer SSE o eliminando las
  cabeceras de actualización de WebSocket.
- El punto de estado se vuelve ámbar mientras se reconecta.

Consulta [Solución de problemas](/operations/troubleshooting/).

## ¿Puedo ver correo HTML de forma segura?

Sí. La página de detalle renderiza el HTML en un iframe en sandbox con los
scripts y el acceso al mismo origen deshabilitados. Los recursos remotos aún
pueden cargarse dentro del marco.

## ¿Están disponibles los adjuntos?

Se conservan dentro del mensaje en bruto. MailVoid no extrae los adjuntos a
archivos o registros separados.

## ¿Puedo buscar en los cuerpos de los mensajes?

No. La búsqueda abarca remitente, destinatario, buzón, dominio, asunto e IP de
origen. La búsqueda de eventos abarca tipo, gravedad y carga útil serializada.

## ¿Cuántos mensajes puede contener?

No hay un límite fijo. La capacidad depende del disco y el coste de las consultas
crece con el tamaño de la tabla porque las búsquedas, los recuentos y los escaneos
de caducidad no están indexados. Usa políticas TTL y vigila el disco.

## ¿Puedo ejecutar varias instancias sobre una sola base de datos?

No. Genji bloquea el directorio. Ejecuta un proceso por directorio de datos. El
escalado horizontal implica instancias separadas y datos separados.

## ¿Puedo eliminar eventos antiguos?

No hay una API para eliminar eventos. El archivo JSONL puede archivarse o
truncarse mientras el servicio está detenido; la tabla `events` de Genji conserva
su propio historial. Planifica un proceso de retención externo si es necesario.

## ¿Hay una imagen de Docker?

No en este repositorio. Compila el binario de la versión y contenedorízalo tú
mismo si es necesario; monta el directorio de datos y define las variables de
entorno.

## ¿Por qué es tan grande el binario?

Incorpora el runtime de Go, Genji/Pebble, la búsqueda de texto completo Bleve, el
CSS completo de DaisyUI/Tailwind (~2.9 MiB), HTMX, Cash y todas las plantillas.
La compilación de versión ocupa aproximadamente 30 MB en linux/amd64;
`mailvoid-it` es un binario separado.

## ¿Funciona en Windows o macOS?

Es código Go sin dependencias específicas de plataforma más allá de las redes y
rutas de archivos estándar. Compílalo desde el código fuente para tu plataforma.
Esta documentación usa rutas de Linux y ejemplos de systemd.

## ¿Por qué Genji?

Es una base de datos de documentos embebida y nativa de Go, sin servicio externo
ni requisito de CGO. La costura del repositorio lo mantiene aislado, de modo que
reemplazar la base de datos no afecta a las capas SMTP, de políticas, de eventos
ni web. Consulta [Almacenamiento](/reference/storage/).

## ¿Puedo usarlo como trampa de spam?

Puedes capturar o descartar correo de fuentes no confiables, pero no hay
puntuación de spam, análisis de virus ni limitación de velocidad. Restringe el
acceso a la red y el uso de disco.

## ¿Cómo comprueban los agentes si llegó correo?

Usa `GET /api/v1/mailboxes/{address}` con un token bearer, o las herramientas MCP
`check_mailbox` y `wait_for_email`. Consulta [OpenAPI](/reference/openapi/) y
[MCP](/reference/mcp/).

## ¿Cómo creo una clave de API o un usuario WebDAV?

Inicia sesión como administrador (o usa el `MAILVOID_API_TOKEN` de arranque) y
llama a `POST /api/v1/admin/api-keys` o `POST /api/v1/admin/webdav-users`. El token
o la contraseña generada se devuelven una sola vez. Consulta
[Credenciales gestionadas](/reference/credentials/).

## ¿Cómo funciona la búsqueda de texto completo?

MailVoid mantiene un índice Bleve embebido sobre asunto, cuerpos, remitente,
destinatario, buzón, dominio y cabeceras. Usa `GET /api/v1/search` o la herramienta
MCP `search_messages`. Consulta [Búsqueda de texto completo](/reference/search/).

## ¿Puedo ejecutarlo en CI?

Sí. `make integration` compila el servidor y el ejecutor autónomo `mailvoid-it`,
inicia MailVoid en puertos temporales y ejecuta la suite validada con JTD.
`make integration-tui` proporciona una vista interactiva. Consulta
[Ejecutor de integración](/development/integration-runner/).

## ¿Cómo descargo archivos .eml?

Usa **Descargar .eml** en la consola, `GET /api/v1/messages/{id}/eml` o el árbol
WebDAV de solo lectura en `/dav/`. Consulta [WebDAV](/operations/webdav/).

## ¿MailVoid admite WebDAV?

Sí, de solo lectura en `/dav/` cuando se define `MAILVOID_WEBDAV_PASSWORD`. Admite
`OPTIONS`, `PROPFIND`, `GET` y `HEAD`; las escrituras devuelven `405`.

## ¿Cómo hago una copia de seguridad?

Detén el servicio y copia todo el directorio de datos, o toma una instantánea
atómica del sistema de archivos. Consulta
[Copia de seguridad y restauración](/operations/backup-restore/).
