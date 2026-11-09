# MailVoid

**Servidor de captura SMTP comodín con panel, políticas, búsqueda, WebDAV y logs.**

[English](README.md) | [Español](README.es.md)

MailVoid es un servidor de captura SMTP de un solo binario, con el evento como
ciudadano de primera clase. Acepta correo para cualquier buzón a través de
múltiples interfaces, puertos y dominios, guarda los metadatos del sobre y el
contenido MIME en una base de datos embebida, aplica reglas de retención con
expresiones regulares ordenadas, mantiene un log tradicional por cada servicio
SMTP y transmite un rastro de auditoría JSON estructurado al panel y a la API.

Está pensado para desarrollo, QA, pruebas de integración e inspección controlada
de correo entrante. No es un MTA, relay, servidor IMAP/POP3 ni proveedor de
buzones de producción.

## Características

- **Captura SMTP multi-interfaz y multi-puerto** — define escuchas como pares
  `host:puerto` o como servicios con nombre y alias.
- **Logs tradicionales por servicio** — cada servicio SMTP escribe su propio log
  de texto en `<data-dir>/logs/<servicio>.log`.
- **Rastro de auditoría basado en eventos** — conexiones, comandos del sobre,
  coincidencias de políticas, almacenamiento, borrados e inicios de sesión se
  convierten en eventos JSON persistidos en la base de datos y en un archivo
  JSONL opcional.
- **Políticas de retención con regex** — conservar, expirar tras un TTL o
  descartar el correo al recibirlo mediante reglas Go RE2 ordenadas. Gana la
  primera coincidencia.
- **Consola web** — panel renderizado en servidor con HTMX, eventos en vivo por
  WebSocket, inspección de mensajes, vista HTML en sandbox y temas claro/oscuro.
- **API REST y OpenAPI** — endpoints versionados en `/api/v1` con autenticación
  por token o sesión y documento OpenAPI 3.1 embebido.
- **Servidor MCP** — Streamable HTTP en `/mcp` para que agentes consulten
  buzones, busquen e inspeccionen mensajes, lean eventos, gestionen políticas y
  obtengan estadísticas.
- **WebDAV de solo lectura** — navega buzones y descarga archivos `.eml` en
  `/dav/` con autenticación Basic.
- **Búsqueda de texto completo embebida** — índice Bleve sobre asuntos, cuerpos,
  remitentes, destinatarios, cabeceras y metadatos.
- **Interruptor de captura en vivo** — pausa y reanuda la captura SMTP desde
  Ajustes; el estado persiste entre reinicios.
- **Administración offline de usuarios** — lista cuentas administradoras y
  restablece contraseñas directamente desde el binario `mailvoid`.
- **Un solo binario** — plantillas, recursos estáticos y el motor de base de
  datos se compilan en un único ejecutable.

## Inicio rápido

Requisitos: Go 1.24.2+ y GNU Make.

```bash
make build
./bin/mailvoid
```

MailVoid carga `./mailvoid.toml` si existe; en caso contrario usa los valores
por defecto. Abre <http://localhost:8025>.

Si no se configura una contraseña de administrador, se genera una aleatoria y se
imprime en el log de arranque. Configúrala explícitamente en `mailvoid.toml` o
con `MAILVOID_ADMIN_PASSWORD`.

Envía un mensaje de prueba al listener SMTP por defecto:

```bash
python3 - <<'PY'
import smtplib

message = (
    "From: sender@example.org\r\n"
    "To: inbox@example.test\r\n"
    "Subject: Hello from MailVoid\r\n"
    "\r\n"
    "This message was captured by MailVoid.\r\n"
)

with smtplib.SMTP("127.0.0.1", 1025) as smtp:
    smtp.sendmail("sender@example.org", ["inbox@example.test"], message)
PY
```

Después abre el panel e inspecciona el mensaje capturado.

## Configuración

La configuración se resuelve en este orden, ganando las fuentes posteriores:

1. Valores por defecto.
2. `mailvoid.toml`.
3. Variables de entorno `MAILVOID_*`.
4. Flags de línea de comandos.

Un `mailvoid.toml` mínimo:

```toml
hostname = "localhost"
scheme = "http"
domains = ["*"]

[web]
bind = "0.0.0.0"
port = 8025

[smtp]
hosts = ["0.0.0.0"]
ports = [1025, 2525]

[events]
json_enabled = true
# json_path = "./data/events.jsonl"

[logging]
enabled = true
# dir = "./data/logs"

[storage]
data_dir = "./data"

[auth]
admin_user = "admin"
admin_password = ""
```

### Servicios SMTP con nombre y logs independientes

Cuando cada listener necesita su propia identidad y su propio log, usa servicios
explícitos:

```toml
[[smtp.services]]
name = "dev"
host = "127.0.0.1"
port = 1025
aliases = ["local", "development"]

[[smtp.services]]
name = "qa"
host = "0.0.0.0"
port = 2525
aliases = ["staging"]
```

Los servicios se escriben en `<logging.dir>/<nombre>.log`. El flujo de eventos
JSON sigue siendo el rastro de auditoría común a todos los servicios.

Consulta [Configuración](docs/es/content/getting-started/configuration.md) para la
referencia completa, incluyendo `public_url`, despliegue tras proxy inverso,
límites de retención y resolución de rutas.

## API, MCP y WebDAV

| Superficie | Ruta | Autenticación |
|------------|------|---------------|
| Panel | `/` | Cookie de sesión |
| API REST | `/api/v1` | Token bearer o cookie de sesión |
| OpenAPI | `/api/v1/openapi.json`, `/api/v1/openapi.yaml` | Pública |
| MCP | `/mcp` | Token bearer |
| WebDAV | `/dav/` | Basic auth |
| Flujo de eventos | WebSocket `/ws/events`, SSE `/api/events/stream` | Sesión o token |

Habilita el token bearer con `MAILVOID_API_TOKEN` / `[auth] api_token`, y WebDAV
con `MAILVOID_WEBDAV_PASSWORD` / `[auth] webdav_password`.

## Desarrollo

```bash
make build             # compila bin/mailvoid
make test              # ejecuta toda la suite de tests
make test-race         # ejecuta los tests con el detector de carreras
make integration       # ejecuta la suite de integración basada en JTD
make lorem             # compila el generador de carga SMTP Lorem Ipsum
make vet               # ejecuta go vet
make fmt               # formatea el código
make assets            # regenera el CSS de Tailwind + DaisyUI (requiere Node.js)
```

Las plantillas usan utilidades de Tailwind y componentes de DaisyUI. El archivo
`internal/web/static/app.css` generado se versiona; ejecuta `make assets` al
cambiar plantillas o estilos.

El generador de correo Lorem vive en `cmd/lorem-mail-generator` y envía mensajes
aleatorios con recuentos de palabras configurables:

```bash
make lorem
./bin/lorem-mail-generator -addr 127.0.0.1:1025 -count 100 \
  -min-words 20 -max-words 200 -random-mailboxes 20 -domains example.test
```

El runner de integración vive en `cmd/mailvoid-it` y también puede ejecutarse en
modo TUI:

```bash
make integration-tui
```

## Estructura del proyecto

```text
cmd/mailvoid/       Binario principal del servidor
cmd/mailvoid-it/    Runner de integración (batch y TUI)
internal/config/    Configuración TOML, entorno y flags
internal/smtpserver/ Listeners SMTP y logs por servicio
internal/storage/   Interfaz de almacenamiento, Genji y backends en memoria
internal/web/       Servidor Fiber, panel, API, MCP y WebDAV
internal/policy/    Motor de retención con regex
internal/events/    Bus de eventos, sink JSONL y tipos de evento
docs/               Sitio de documentación Hugo
testdata/           Definiciones de tests de integración
```

## Documentación

- [Introducción](docs/es/content/overview/introduction.md)
- [Características](docs/es/content/overview/features.md)
- [Configuración](docs/es/content/getting-started/configuration.md)
- [Inicio rápido](docs/es/content/getting-started/quick-start.md)
- [Referencia de la API](docs/es/content/reference/api.md)
- [MCP](docs/es/content/reference/mcp.md)
- [Eventos](docs/es/content/reference/events.md)
- [Despliegue](docs/es/content/operations/deployment.md)

> [!WARNING]
> MailVoid acepta todos los dominios por defecto y no implementa autenticación
> SMTP ni STARTTLS. Nunca expongas un listener sin restricciones a internet.
> Restringe los dominios, enlaza a interfaces de confianza y bloquea los puertos
> SMTP en el firewall.

## Licencia

MIT License - Copyright (c) 2026 Antonio Ognio and contributors

Made with ❤️ from 🇵🇪. El Perú es clave 🔑.
