---
title: Documentación
bookToC: true
---

# MailVoid

MailVoid es un servidor de captura SMTP de binario único y centrado en eventos
con una consola administrativa integrada. Acepta correo para cualquier buzón en
múltiples puertos y dominios, almacena el sobre y el contenido MIME en una base
de datos Genji embebida, aplica reglas ordenadas de retención mediante
expresiones regulares y transmite un rastro de auditoría JSON estructurado al
navegador en tiempo real.

{{% columns %}}
- **Captura comodín**  
  Escucha en varios puertos SMTP a la vez y acepta cualquier buzón local para
  cada dominio configurado.

- **Núcleo centrado en eventos**  
  Las conexiones, los comandos de sobre, las coincidencias de políticas, el
  almacenamiento, las eliminaciones y los inicios de sesión se convierten en
  eventos estructurados que se persisten en Genji y en un archivo JSONL de solo
  adición.

- **Retención con expresiones regulares**  
  Conserva, caduca tras un TTL o descarta el correo a su llegada usando reglas
  Go RE2 ordenadas. Reordena y prueba reglas desde el panel o la API JSON.

- **Un solo binario**  
  Fiber, las plantillas de Go, HTMX, DaisyUI/Tailwind, Cash, la base de datos
  Genji y todos los recursos se compilan en un único ejecutable estático.
{{% /columns %}}

{{< button href="/getting-started/quick-start/" >}}Inicio rápido{{< /button >}}
{{< button href="/overview/architecture/" >}}Arquitectura{{< /button >}}
{{< button href="/reference/api/" >}}Referencia de la API{{< /button >}}

## De un vistazo

| Propiedad | Valor |
|-----------|-------|
| Lenguaje | Go 1.24.2+ |
| Framework web | Fiber v2 |
| Base de datos | Genji v0.15.3 (embebida, respaldada por Pebble) |
| Puertos SMTP predeterminados | `1025`, `2525` |
| Panel predeterminado | `http://localhost:8025` |
| Directorio de datos predeterminado | `./data` |
| Autenticación | credenciales bcrypt + sesiones por cookie |
| Administrador predeterminado | `admin` con una contraseña generada cuando no se configura ninguna |
| Transportes de eventos | archivo JSONL, tabla Genji, Server-Sent Events, WebSocket |
| API para agentes | `/api/v1` con OpenAPI 3.1 y autenticación bearer/sesión |
| Herramientas para agentes | MCP Streamable HTTP en `/mcp` |
| Acceso a archivos | descargas `.eml` y WebDAV de solo lectura en `/dav/` |
| Búsqueda | índice de texto completo Bleve embebido en `/api/v1/search` |
| Gestión de credenciales | API de administrador para claves de API y usuarios WebDAV |
| Ejecutor de integración | cliente TUI/por lotes `mailvoid-it` con archivos de prueba JTD |

## Adónde ir a continuación

- ¿Eres nuevo en MailVoid? Empieza con [Instalación](/getting-started/installation/)
  y el [Inicio rápido](/getting-started/quick-start/).
- ¿Vas a operarlo? Lee [Configuración](/getting-started/configuration/),
  [Despliegue](/operations/deployment/) y [Seguridad](/operations/security/).
- ¿Vas a desarrollar contra él? Lee la [Referencia de la API](/reference/api/),
  [OpenAPI](/reference/openapi/), [Servidor MCP](/reference/mcp/) y la
  [Referencia de eventos](/reference/events/).
- ¿Vas a conectar agentes o clientes de archivos? Lee [MCP](/reference/mcp/) y
  [WebDAV](/operations/webdav/).
- ¿Vas a trabajar en el código? Lee [Estructura del proyecto](/development/project-layout/)
  y [Pruebas](/development/testing/).

> [!NOTE]
> MailVoid es un dispositivo de captura, no un agente de transferencia de
> correo. Nunca retransmite correo saliente y no implementa autenticación SMTP ni
> STARTTLS. Vincúlalo a una red de confianza o colócalo detrás de un cortafuegos.
