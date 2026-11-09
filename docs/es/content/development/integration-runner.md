---
title: Ejecutor de integración
weight: 50
---

# Ejecutor de integración

`mailvoid-it` es un binario Go independiente que lanza MailVoid y ejercita la API
REST, las herramientas MCP, la captura SMTP, las credenciales gestionadas, las
descargas `.eml`, WebDAV y la búsqueda de texto completo.

Tiene dos modos:

- **Interactivo** — una TUI de Charmbracelet Bubble Tea para seleccionar y
  ejecutar pruebas.
- **Por lotes** — un ejecutor apto para CI controlado por archivos JSON validados
  con JTD.

El ejecutor es independiente de los paquetes de la aplicación: solo se comunica
por HTTP, SMTP, MCP y WebDAV.

## Compilación y ejecución

```bash
make it                 # build bin/mailvoid-it
make integration        # build both, run the full suite in batch mode
make integration-tui    # open the interactive runner
```

La suite por defecto vive en `testdata/integration`:

```text
testdata/integration/
├── schema.jtd.json
├── 01-credentials.json
├── 02-mail-flow.json
└── 03-search.json
```

`make integration` lanza `bin/mailvoid` con un directorio de datos temporal,
puertos locales libres, búsqueda habilitada, sin credenciales de bootstrap para
API/WebDAV y la contraseña de administrador `mailvoid-it-password`, salvo que se
sobrescriba.

## CLI

| Flag | Valor por defecto | Significado |
|------|-------------------|-------------|
| `-suite` | `testdata/integration` | Directorio que contiene las pruebas JTD |
| `-server` | `bin/mailvoid` | Binario del servidor que se lanzará |
| `-base-url` | *(vacío)* | Usa un endpoint HTTP externo en lugar de lanzar el servidor |
| `-smtp-addr` | *(vacío)* | `host:port` SMTP externo; obligatorio con `-base-url` |
| `-admin-user` | `admin` | Nombre de usuario del administrador |
| `-admin-password` | `mailvoid-it-password` | Contraseña del administrador |
| `-batch` | `false` | Desactiva la TUI |
| `-validate-only` | `false` | Valida los archivos JTD y sale |
| `-list` | `false` | Lista las pruebas y sale |
| `-test` | *(vacío)* | Filtro de nombres/archivos separados por comas |
| `-verbose` | `false` | Muestra todos los registros de pasos en modo por lotes |
| `-keep` | `false` | Conserva el directorio de datos del servidor lanzado |

Cuando stdout no es un terminal, el ejecutor usa automáticamente el modo por
lotes.

## TUI interactiva

Teclas:

| Tecla | Acción |
|-------|--------|
| `↑` / `↓` o `j` / `k` | Mover la selección |
| `space` | Alternar una prueba |
| `a` | Seleccionar todas / ninguna |
| `enter` | Ejecutar las pruebas seleccionadas (o repetir desde la vista de resultados) |
| `q` / `ctrl-c` | Salir |

La TUI muestra:

- la URL del servidor y el directorio de datos temporal;
- la selección por prueba y el estado de éxito/fallo;
- un spinner y un contador de completadas/total mientras se ejecuta;
- glifos de estado `✔` / `✖` con colores semánticos;
- un panel de registro de pasos desplazable para la prueba resaltada.

Usa `bubbletea`, `lipgloss`, `bubbles/spinner` y `bubbles/viewport` del ecosistema
Charmbracelet. La salida por lotes recurre a etiquetas simples cuando stdout no es
un TTY o cuando `NO_COLOR` está definido.

## Archivos de prueba JTD

Cada archivo de prueba es un documento JSON validado contra `schema.jtd.json`
(JSON Type Definition, RFC 8927) antes de ejecutarse. Un archivo contiene un
nombre, una descripción opcional y una lista ordenada de pasos.

```json
{
  "name": "mail delivery surfaces",
  "description": "Send SMTP and verify delivery over REST, MCP, and WebDAV.",
  "steps": [
    {
      "type": "smtp",
      "name": "send mail",
      "from": "sender@example.org",
      "to": ["inbox-${run_id}@example.test"],
      "subject": "Delivery ${run_id}",
      "text": "unique body ${run_id}",
      "port": "${smtp_port}"
    },
    {
      "type": "http",
      "name": "check mailbox",
      "method": "GET",
      "path": "/api/v1/mailboxes/inbox-${run_id}@example.test",
      "bearer": "${api_key}",
      "save": { "message_id": "latest_message_id" },
      "expect": {
        "status": 200,
        "json": [{ "path": "message_count", "equals": 1 }]
      }
    }
  ]
}
```

### Tipos de paso

| Tipo | Campos obligatorios | Notas |
|------|---------------------|-------|
| `http` | `method`, `path` | `body` o `body_raw` en JSON; `auth: session`, `bearer` o `basic` |
| `webdav` | `method`, `path`, `basic` | `depth`, `body_raw` XML |
| `mcp` | `tool` | `arguments`; autenticación `bearer` o de sesión |
| `smtp` | `from`, `to` | `subject`, `text`, `html`, `port` |
| `sleep` | `millis` | Pausa entre pasos |

### Variables

Integradas: `${run_id}`, `${base_url}`, `${smtp_host}`, `${smtp_port}`,
`${smtp_port2}`, `${admin_user}`, `${admin_password}`, `${domain}`.

El mapa `save` guarda valores de JSON HTTP o de resultados estructurados de MCP:

```json
"save": { "api_key": "token", "key_id": "id", "message_id": "messages.0.id" }
```

Los segmentos de ruta se separan por puntos; los segmentos numéricos indexan
arrays. Las variables no definidas o las rutas de guardado ausentes hacen fallar
el paso.

### Aserciones

`expect` admite:

| Campo | Significado |
|-------|-------------|
| `status` | Código de estado HTTP exacto |
| `body_contains`, `body_not_contains` | Subcadenas del cuerpo sin procesar |
| `headers` | Comprobaciones de subcadenas en cabeceras |
| `json` | Una lista de aserciones de ruta |

Cada aserción JSON puede usar `exists`, `equals`, `contains`, `gte`, `lte` y
`length`. Las aserciones se ejecutan contra el JSON HTTP analizado o la salida
estructurada de MCP.

## Aislamiento y limpieza

Cada prueba obtiene un tarro de cookies de administrador nuevo y su propio
`run_id`. Usa el ID de ejecución en los nombres de buzón, en los nombres de claves
de API y en los nombres de usuario de WebDAV. Las pruebas eliminan sus credenciales
y mensajes; el servidor lanzado usa un directorio de datos temporal que se elimina
salvo que se defina `-keep`.

## Añadir una prueba

1. Añade `testdata/integration/NN-name.json`.
2. Valida las formas con `mailvoid-it -validate-only`.
3. Ejecuta una prueba con `-test name` en modo por lotes o selecciónala en la TUI.
4. Prefiere direcciones con ámbito `${run_id}` para que las pruebas sigan siendo
   independientes.
5. Nunca pongas secretos reales en el JSON; crea credenciales mediante la sesión
   de administrador y guarda el token devuelto en una variable.
