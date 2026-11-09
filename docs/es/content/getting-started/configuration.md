---
title: Configuración
weight: 30
---

# Configuración

MailVoid resuelve la configuración en este orden:

1. Valores predeterminados integrados.
2. El archivo de configuración `mailvoid.toml`.
3. Variables de entorno.
4. Flags de línea de comandos.

Las fuentes posteriores tienen prioridad sobre las anteriores. Usa
`-config /path/to/mailvoid.toml` o `MAILVOID_CONFIG` para cargar un archivo
específico. Sin ninguno de los dos, se carga `./mailvoid.toml` cuando existe.

## Archivo de configuración

Un `mailvoid.toml` completo tiene este aspecto:

```toml
hostname = "mail.example.com"
scheme = "https"
# public_url = "https://mail.example.com"   # overrides hostname/scheme/web.port
domains = ["*"]

[web]
bind = "127.0.0.1"
port = 8025

[smtp]
hosts = ["127.0.0.1", "10.0.0.5"]
ports = [1025, 2525]

# Optional explicit services. Each entry is one interface:port pair with its
# own identity and traditional log file. When present, services replace the
# simple hosts/ports form above.
[[smtp.services]]
name = "dev"
host = "127.0.0.1"
port = 1025
aliases = ["local", "development"]
# log_path = "/var/log/mailvoid/dev.log"

[[smtp.services]]
name = "qa"
host = "0.0.0.0"
port = 2525
aliases = ["staging"]

[events]
json_enabled = true
json_path = "./data/events.jsonl"

[logging]
enabled = true
dir = "./data/logs"

[storage]
data_dir = "./data"
# db_path = "./data/mailvoid.db"

[search]
enabled = true
# index_path = "./data/search.bleve"

[auth]
admin_user = "admin"
admin_password = ""
api_token = ""
webdav_user = "mailvoid"
webdav_password = ""
cookie_secure = false

[limits]
max_message_bytes = 26214400
read_timeout = "60s"
sweep_interval = "30s"
```

Las claves desconocidas se rechazan al arrancar, por lo que los errores
tipográficos fallan rápido.

## Interfaces, puertos y servicios SMTP

Hay dos formas equivalentes de describir los listeners SMTP.

### Forma simple

```toml
[smtp]
hosts = ["127.0.0.1", "10.0.0.5"]
ports = [1025, 2525]
```

Esto enlaza cada interfaz de `hosts` en cada puerto de `ports`. Las variables de
entorno heredadas `MAILVOID_SMTP_HOST`/`MAILVOID_SMTP_PORTS` y los flags
`-smtp-host`/`-smtp-ports` usan la misma forma de host único.

### Servicios explícitos

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

Cada servicio es una interfaz y un puerto. Se requiere que `name` sea único;
`aliases` son nombres alternativos y también deben ser únicos. Si se omite
`name`, se usa el primer alias y, luego, un nombre generado como `smtp-1025` o
`smtp-127.0.0.1-2525`.

Cada servicio escribe un log de texto tradicional en
`<logging.dir>/<service-name>.log` (o en `log_path` cuando está configurado).
Los eventos de ese listener incluyen un campo `service`, mientras que el log de
eventos JSONL sigue siendo el rastro de auditoría entre servicios.

`MAILVOID_SMTP_HOSTS` acepta una lista de interfaces separadas por comas.
Establecer `MAILVOID_SMTP_HOST` o `MAILVOID_SMTP_PORTS` colapsa los listeners de
nuevo a la forma simple de host único y regenera los nombres de servicio.

## Variables de entorno

| Variable | Predeterminado | Descripción |
|----------|----------------|-------------|
| `MAILVOID_CONFIG` | `./mailvoid.toml` | Ruta del archivo de configuración. |
| `MAILVOID_HOSTNAME` | `localhost` | Nombre de host público usado para enlaces absolutos. |
| `MAILVOID_SCHEME` | `http` | Esquema de URL público: `http` o `https`. |
| `MAILVOID_PUBLIC_URL` | *(vacío)* | Anulación completa de la URL base pública usada para los enlaces generados. |
| `MAILVOID_SMTP_HOST` | `0.0.0.0` | Interfaz de escucha SMTP única. Usa `127.0.0.1` para captura solo local. |
| `MAILVOID_SMTP_HOSTS` | *(vacío)* | Interfaces de escucha SMTP separadas por comas (forma simple multiinterfaz). |
| `MAILVOID_SMTP_PORTS` | `1025,2525` | Puertos SMTP separados por comas o espacios. Se eliminan los duplicados. |
| `MAILVOID_HTTP_ADDR` | `:8025` | Dirección de escucha HTTP. `:8025` enlaza todas las interfaces; `127.0.0.1:8025` enlaza loopback. |
| `MAILVOID_WEB_BIND` | *(desde HTTP_ADDR)* | Interfaz de escucha HTTP, aplicada después de `MAILVOID_HTTP_ADDR`. |
| `MAILVOID_WEB_PORT` | *(desde HTTP_ADDR)* | Puerto de escucha HTTP, aplicado después de `MAILVOID_HTTP_ADDR`. |
| `MAILVOID_DOMAINS` | `*` | Dominios de destinatario aceptados. `*` acepta todos; se admiten nombres exactos y patrones `*.zone`. |
| `MAILVOID_DATA_DIR` | `./data` | Directorio base para la base de datos, el log de eventos, los logs y el índice de búsqueda. Se crea si falta. |
| `MAILVOID_DB_PATH` | `<data-dir>/mailvoid.db` | Directorio de la base de datos Genji. |
| `MAILVOID_EVENT_LOG` | `<data-dir>/events.jsonl` | Ruta del log de eventos JSONL de solo anexado. |
| `MAILVOID_EVENT_LOG_ENABLED` | `true` | Habilita el log de eventos JSONL. |
| `MAILVOID_LOG_ENABLED` | `true` | Habilita los logs de texto tradicionales por servicio. |
| `MAILVOID_LOG_DIR` | `<data-dir>/logs` | Directorio para los logs por servicio. |
| `MAILVOID_ADMIN_USER` | `admin` | Nombre de usuario del administrador de arranque (se almacena en minúsculas). |
| `MAILVOID_ADMIN_PASSWORD` | *(vacío)* | Contraseña de arranque. Si está vacía en la primera creación, se genera una contraseña aleatoria y se registra una vez. |
| `MAILVOID_ADMIN_RESET_PASSWORD` | `false` | Cuando es `true`, restablece la contraseña existente del administrador al valor configurado/generado. |
| `MAILVOID_SWEEP_INTERVAL` | `30s` | Intervalo del barrido TTL. Cualquier duración de Go (`1s`, `5m`, `1h`). |
| `MAILVOID_MAX_MESSAGE_BYTES` | `26214400` | Tamaño máximo de mensaje aceptado (25 MiB). |
| `MAILVOID_READ_TIMEOUT` | `60s` | Tiempo de espera de lectura y escritura SMTP. |
| `MAILVOID_COOKIE_SECURE` | `false` | Marca la cookie de sesión como `Secure`. Habilítalo cuando se sirva por HTTPS. |
| `MAILVOID_API_TOKEN` | *(vacío)* | Token Bearer aceptado por `/api/v1` y `/mcp`. Vacío deshabilita la autenticación Bearer. |
| `MAILVOID_WEBDAV_USER` | `mailvoid` | Nombre de usuario de autenticación Basic para `/dav/`. |
| `MAILVOID_WEBDAV_PASSWORD` | *(vacío)* | Contraseña de autenticación Basic para `/dav/`. Vacío deshabilita la credencial WebDAV de arranque. |
| `MAILVOID_SEARCH_ENABLED` | `true` | Habilita el índice de texto completo Bleve embebido. |
| `MAILVOID_SEARCH_INDEX` | `<data-dir>/search.bleve` | Directorio del índice Bleve. |

## Flags

Los flags usan los mismos nombres con un `-` inicial (el paquete flag de Go
también acepta `--`):

| Flag | Equivalente |
|------|-------------|
| `-config` | `MAILVOID_CONFIG` |
| `-hostname` | `MAILVOID_HOSTNAME` |
| `-scheme` | `MAILVOID_SCHEME` |
| `-public-url` | `MAILVOID_PUBLIC_URL` |
| `-smtp-host` | `MAILVOID_SMTP_HOST` |
| `-smtp-hosts` | `MAILVOID_SMTP_HOSTS` |
| `-smtp-ports` | `MAILVOID_SMTP_PORTS` |
| `-http-addr` | `MAILVOID_HTTP_ADDR` |
| `-web-bind` | `MAILVOID_WEB_BIND` |
| `-web-port` | `MAILVOID_WEB_PORT` |
| `-domains` | `MAILVOID_DOMAINS` |
| `-data-dir` | `MAILVOID_DATA_DIR` |
| `-db-path` | `MAILVOID_DB_PATH` |
| `-event-log` | `MAILVOID_EVENT_LOG` |
| `-event-log-enabled` | `MAILVOID_EVENT_LOG_ENABLED` |
| `-log-enabled` | `MAILVOID_LOG_ENABLED` |
| `-log-dir` | `MAILVOID_LOG_DIR` |
| `-admin-user` | `MAILVOID_ADMIN_USER` |
| `-admin-password` | `MAILVOID_ADMIN_PASSWORD` |
| `-admin-reset-password` | `MAILVOID_ADMIN_RESET_PASSWORD` |
| `-sweep-interval` | `MAILVOID_SWEEP_INTERVAL` |
| `-max-message-bytes` | `MAILVOID_MAX_MESSAGE_BYTES` |
| `-read-timeout` | `MAILVOID_READ_TIMEOUT` |
| `-cookie-secure` | `MAILVOID_COOKIE_SECURE` |
| `-api-token` | `MAILVOID_API_TOKEN` |
| `-webdav-user` | `MAILVOID_WEBDAV_USER` |
| `-webdav-password` | `MAILVOID_WEBDAV_PASSWORD` |
| `-search-enabled` | `MAILVOID_SEARCH_ENABLED` |
| `-search-index` | `MAILVOID_SEARCH_INDEX` |

`--version` imprime la versión, el commit y la fecha de compilación y sale. Se
maneja antes de cargar la configuración.

## Resolución de rutas

Después de fusionar todas las fuentes:

- Si `data-dir` está vacío, pasa a ser `./data`.
- Si `db-path` está vacío, pasa a ser `<data-dir>/mailvoid.db`.
- Si `event-log` está vacío, pasa a ser `<data-dir>/events.jsonl`.
- Si `log-dir` está vacío, pasa a ser `<data-dir>/logs`.
- Si `search-index` está vacío, pasa a ser `<data-dir>/search.bleve`.
- Si `sweep-interval` es cero o negativo, pasa a ser `30s`.
- Si `max-message-bytes` es cero o negativo, pasa a ser 25 MiB.
- Si `read-timeout` es cero o negativo, pasa a ser `60s`.

## Enlaces públicos

`hostname`, `scheme` y el puerto web definen el prefijo absoluto usado para los
enlaces generados. Por ejemplo, `hostname = "mail.example.com"`, `scheme =
"https"` y `[web] port = 8025` producen `https://mail.example.com:8025`.

Establece `public_url` cuando MailVoid se ejecuta detrás de un proxy inverso y la
URL pública difiere de la dirección de escucha:

```toml
public_url = "https://mail.example.com"
```

## Coincidencia de dominios

Los dominios de destinatario se convierten a minúsculas y se eliminan los puntos
finales. Un dominio se acepta cuando coincide cualquier entrada:

| Entrada | Coincide |
|---------|----------|
| `*` | Cualquier dominio |
| `example.test` | Exactamente `example.test` |
| `*.example.test` | `example.test` y cualquier subdominio como `mail.example.test` |

Si la lista de dominios está vacía, se acepta cualquier dominio. El valor
predeterminado es `*`.

> [!WARNING]
> Aceptar todos los dominios (`*` o una lista vacía) convierte a MailVoid en un
> sumidero SMTP universal. Restringe la lista de dominios y la exposición de red
> en cualquier entorno compartido.

## Arranque del administrador

Al arrancar:

1. El nombre de usuario configurado se convierte a minúsculas. El predeterminado
   es `admin`; un nombre de usuario explícitamente vacío falla la validación
   antes del arranque.
2. Si el usuario no existe, se crea con la contraseña configurada.
3. Si la contraseña está vacía, se genera una contraseña aleatoria de 24
   caracteres segura para URL y se imprime en el log una vez.
4. Si el usuario existe, la contraseña no se cambia a menos que
   `MAILVOID_ADMIN_RESET_PASSWORD=true`.
5. Con el restablecimiento habilitado, se aplica la contraseña configurada; si no
   hay contraseña configurada, se genera e imprime una nueva contraseña
   aleatoria.

Las contraseñas se almacenan con hash bcrypt con coste 12. Nunca se almacenan en
texto plano.

## Reglas de validación

El arranque falla si:

- no hay servicios SMTP configurados;
- un puerto SMTP está fuera de `1–65535`;
- dos servicios SMTP comparten nombre o alias;
- la dirección HTTP está vacía;
- el nombre de usuario del administrador está vacío;
- `scheme` no es `http` o `https`;
- `public_url` no es una URL HTTP(S) absoluta;
- el archivo TOML contiene claves desconocidas.

Duraciones, booleanos, recuentos de bytes o cadenas de puerto no válidos fallan
durante la carga de la configuración con un error que nombra el valor infractor.

## Ejemplos

### Solo desarrollo local

```toml
hostname = "localhost"
scheme = "http"

[web]
bind = "127.0.0.1"
port = 8025

[smtp]
hosts = ["127.0.0.1"]
ports = [1025, 2525]
domains = ["*.test", "localhost"]

[auth]
admin_password = "dev-only-password"
```

### Varios proyectos en un mismo host

```toml
[web]
bind = "127.0.0.1"
port = 8025

[[smtp.services]]
name = "app"
host = "127.0.0.1"
port = 1025
aliases = ["app.local"]

[[smtp.services]]
name = "api"
host = "127.0.0.1"
port = 2525
aliases = ["api.local"]

[[smtp.services]]
name = "external"
host = "0.0.0.0"
port = 3025
aliases = ["mail.local"]

[storage]
data_dir = "/var/lib/mailvoid"

[auth]
admin_password = "use-a-secret-manager"
```

Cada servicio registra en su propio archivo, por ejemplo
`/var/lib/mailvoid/logs/app.log` y `/var/lib/mailvoid/logs/api.log`.

### Detrás de un proxy inverso TLS con agentes

```toml
hostname = "mail.example.com"
scheme = "https"
public_url = "https://mail.example.com"

[web]
bind = "127.0.0.1"
port = 8025

[auth]
cookie_secure = true
api_token = "use-a-secret-manager"
webdav_user = "mailvoid"
webdav_password = "another-secret"
admin_password = "use-a-secret-manager"
```

Termina TLS en el proxy y habilita `cookie_secure`. El token de API habilita
`/api/v1` y `/mcp`; las credenciales WebDAV habilitan `/dav/` de solo lectura.
Consulta [Proxy inverso](/operations/reverse-proxy/), [OpenAPI](/reference/openapi/),
[MCP](/reference/mcp/) y [WebDAV](/operations/webdav/).

## El directorio de datos

| Ruta | Contenido |
|------|-----------|
| `<data-dir>/mailvoid.db/` | Directorio de base de datos Genji/Pebble (mensajes, políticas, eventos, usuarios, credenciales) |
| `<data-dir>/events.jsonl` | Log de eventos JSON de solo anexado, un objeto por línea |
| `<data-dir>/logs/<service>.log` | Log de texto tradicional para cada servicio SMTP |
| `<data-dir>/search.bleve/` | Índice de texto completo Bleve reconstruible |

El directorio de la base de datos contiene un archivo `LOCK`. No ejecutes dos
procesos de MailVoid contra el mismo directorio de datos.

Consulta [Copia de seguridad y restauración](/operations/backup-restore/) para
procedimientos de copia seguros.
