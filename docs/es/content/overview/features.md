---
title: Características
weight: 20
---

# Características

## Captura SMTP

- Varios listeners SMTP simultáneos en puertos configurables.
- Aceptación de buzones comodín con políticas de dominio exactas, comodín de
  zona y aceptar todo.
- Captura del sobre por mensaje: IP de origen, puerto receptor, dominio y buzón
  del destinatario, remitente del sobre, marca de tiempo y tamaño.
- Retención del mensaje en bruto más asunto decodificado, cuerpo en texto plano
  y cuerpo HTML.
- Manejo MIME para mensajes multiparte, base64, quoted-printable, asuntos
  codificados RFC 2047 y conjuntos de caracteres comunes.
- Un mensaje almacenado por cada destinatario del sobre, de modo que una política
  puede tratar `alice@example.test` de forma distinta a
  `bob@example.test`.
- Manejo tolerante de mensajes no analizables: los bytes en bruto se almacenan
  igualmente y se emite un evento de advertencia.

## Políticas de retención

- Reglas regex ordenadas con evaluación de primera coincidencia gana.
- Campos objetivo: `sender`, `recipient`, `mailbox`, `domain`, `subject`,
  `remote_ip`, `local_port` y `any`.
- Acciones: `keep` de forma permanente, `ttl` con eliminación automática tras una
  duración, y `drop` como agujero negro al llegar.
- Sintaxis Go RE2: coincidencia en tiempo lineal, sin retroreferencias ni
  lookaround.
- Crear, editar, habilitar/deshabilitar, reordenar, eliminar y probar reglas
  desde la UI o la API JSON.
- El barrido TTL elimina los mensajes caducados y emite `message.deleted` con
  motivo `ttl expiry`.

## API, MCP y acceso a archivos

- API REST versionada en `/api/v1` con un documento OpenAPI 3.1.0 embebido.
- Autenticación por token Bearer para agentes y automatización.
- Servidor MCP en `/mcp` sobre Streamable HTTP sin estado, con herramientas para
  comprobaciones de buzones, esperas de entrega, inspección de mensajes,
  búsqueda, eventos, políticas, estadísticas y eliminación.
- Descargas de `.eml` en bruto desde la página de detalle del mensaje y
  `GET /api/v1/messages/{id}/eml`.
- WebDAV de solo lectura en `/dav/` con autenticación Basic: lista buzones y
  descarga archivos `<message-id>.eml` desde cualquier cliente WebDAV.

## Búsqueda de texto completo

- Índice Bleve embebido sobre asunto, cuerpos decodificados, remitente,
  destinatario, buzón, dominio, IP de origen y cabeceras.
- Clasificación BM25, resaltados, filtros de metadatos y paginación mediante
  `GET /api/v1/search` y la herramienta `search_messages` de MCP.
- Proyección reconstruible de Genji; los fallos se reportan como `search.error`
  y no bloquean la captura de correo.
- La búsqueda de subcadenas en metadatos permanece como alternativa cuando el
  índice no está disponible.

## Administración offline de usuarios

- Lista cuentas administradoras con `mailvoid users list`.
- Restablece la contraseña de un administrador con `mailvoid users passwd [username]`.
- Los comandos abren la base de datos directamente, así que detén primero el
  servidor; la base de datos usa un bloqueo exclusivo.

## Credenciales gestionadas

- Endpoints de administración de la API para crear, listar y revocar claves de
  API y usuarios WebDAV.
- Las claves de API se muestran una sola vez y se almacenan como hashes SHA-256.
- Las contraseñas WebDAV se almacenan con hash bcrypt y se generan cuando se
  omiten.
- Las credenciales gestionadas surten efecto de inmediato; no se requiere
  reiniciar.

## Sistema de eventos

- Eventos estructurados con ID, marca de tiempo UTC, tipo, severidad y carga útil
  JSON.
- Persistidos tanto en un archivo JSONL de solo anexado como en una tabla
  `events` de Genji.
- Difundidos a navegadores por WebSocket con una alternativa SSE para scripts.
- Búfer circular en memoria acotado para eventos muy recientes.
- Cobertura de eventos para el ciclo de vida del sistema, el ciclo de vida SMTP,
  el ciclo de vida de los mensajes, las políticas y la autenticación.

## Consola web

- Páginas renderizadas en el servidor con actualizaciones parciales HTMX.
- Estilos DaisyUI/Tailwind con un conmutador persistente de tema claro/oscuro.
- Panel con recuentos, mensajes recientes y eventos recientes.
- Búsqueda de mensajes y filtros por dominio, buzón, puerto y acción.
- Detalle del mensaje con vista previa HTML en sandbox, cuerpo de texto,
  cabeceras, mensaje en bruto, política coincidente y una cuenta atrás de
  caducidad en vivo.
- Panel de gestión de políticas con crear, editar, alternar, reordenar, eliminar
  y un probador de regex.
- Flujo de eventos en vivo con controles de pausa y limpieza.
- Copia al portapapeles con Cash, cambio de tema y comportamiento de cajón en
  móvil.

## Autenticación y administración

- Hashes de contraseña bcrypt (coste 12).
- Sesiones basadas en cookies con HTTP-only y SameSite=Lax por defecto.
- Un administrador de arranque, configurable mediante variables de entorno o
  flags; se genera y se imprime una contraseña aleatoria una sola vez si no se
  proporciona ninguna.
- Cambio de contraseña desde la página de ajustes.
- Cada cuenta autenticada tiene acceso completo de administrador.
- Todas las rutas de la UI y los endpoints de la API están protegidos, excepto
  `/healthz`, `/login` y los recursos estáticos embebidos.

## Ejecutor de integración

- Binario `mailvoid-it` independiente que lanza MailVoid y ejercita REST, MCP,
  SMTP, WebDAV, `.eml`, credenciales y búsqueda.
- TUI interactiva de Charmbracelet y modo por lotes apto para CI.
- Las pruebas son archivos JSON validados con JTD bajo `testdata/integration`.

## Operaciones

- Binario estático único; sin Node y sin requisito de CGO para compilaciones de
  release.
- Base de datos y recursos embebidos.
- Apagado ordenado de los listeners HTTP, SSE, WebSocket y SMTP.
- Configuración mediante variables de entorno y flags.
- API JSON para mensajes, eventos, políticas, estadísticas y pruebas de regex.
- Persistencia entre reinicios.
