---
title: Limitaciones
weight: 70
---

# Limitaciones

MailVoid es intencionalmente acotado. Estas son restricciones conocidas, no
errores.

## SMTP

- **Sin STARTTLS ni TLS implícito.** El tráfico SMTP es en texto plano.
- **Sin SMTP AUTH.** Cualquiera que pueda alcanzar un listener puede enviar
  correo.
- **Sin retransmisión ni entrega saliente.** MailVoid nunca reenvía correo.
- **No se anuncian SMTPUTF8, DSN ni otras extensiones opcionales.** Se usa la
  transacción base de go-smtp.
- **Sin limitación de velocidad por cliente ni límites de conexión.**
- **No existe un evento de handshake TLS** porque TLS no está implementado. El
  modelo de eventos cubre cada acción del ciclo de vida que el servidor realiza
  realmente.
- **Una copia almacenada por cada destinatario del sobre.** El correo grande a
  muchos destinatarios multiplica el almacenamiento.

## MIME

- Los adjuntos se conservan solo dentro del mensaje en bruto; no se extraen a
  registros separados.
- Las partes `message/rfc822` no se analizan de forma recursiva.
- El juego de caracteres se toma del parámetro MIME; se ignora `<meta charset>`
  del HTML.
- Solo se decodifican los cuerpos text/plain y text/html.
- No se realiza saneamiento del HTML almacenado. En su lugar, la consola lo aísla
  en un iframe en sandbox.

## Búsqueda

- La búsqueda es una coincidencia de subcadena sobre metadatos (y cargas útiles
  de eventos), no una búsqueda de texto completo sobre los cuerpos.
- El repositorio Genji usa `LIKE` sin índices secundarios; las tablas grandes
  hacen más lentas las búsquedas y los recuentos.
- `%` y `_` conservan su significado de comodín SQL en las consultas.

## Eventos

- El historial de eventos y el de JSONL son de solo adición y no tienen retención
  ni compactación integradas.
- Los transportes en vivo no admiten reproducción ni `Last-Event-ID`.
- Los fallos del sumidero de eventos se ignoran para que un fallo de auditoría no
  pueda bloquear la captura.
- Los suscriptores lentos en vivo descartan eventos en lugar de aplicar
  contrapresión.
- No hay una API para eliminar eventos.

## Búsqueda

- La búsqueda de texto completo usa Bleve v2.5.7, la versión más reciente
  compatible con Go 1.24; las versiones más nuevas de Bleve requieren Go 1.25.
- El MIME en bruto, las cargas útiles base64 y el contenido de los adjuntos no se
  indexan. El HTML se reduce a texto.
- El índice es una proyección síncrona. Los fallos de indexación no bloquean la
  captura, pero la búsqueda puede quedar desactualizada hasta una reconstrucción.
- Cuando el índice está deshabilitado o no disponible, `/search` y MCP recurren a
  la búsqueda heredada por subcadena de metadatos, que no busca en los cuerpos.

## API y superficies para agentes

- `/api/v1` y `/mcp` comparten un único token bearer global. No hay ámbitos,
  tokens ni identidades de auditoría por agente más allá de `api-token`.
- MCP no tiene estado y devuelve respuestas JSON; no expone transmisión de
  eventos ni herramientas de modificación de políticas.
- WebDAV es de solo lectura y usa una única credencial Basic-auth global. No
  tiene autorización por buzón, bloqueo ni escritura de propiedades.
- Las descargas `.eml` devuelven los bytes en bruto almacenados; los adjuntos no
  se extraen por separado.

## Autenticación y administración

- Cada cuenta autenticada es administradora; no hay roles ni permisos por buzón.
- No hay interfaz ni API de gestión de usuarios para crear cuentas adicionales.
- Las sesiones se almacenan en memoria y se restablecen al reiniciar.
- No se implementan tokens CSRF; la cookie de sesión es `SameSite=Lax`.
- Sin limitación de velocidad de inicio de sesión, bloqueo ni auditoría de la
  fortaleza de las contraseñas.
- Sin tokens de API; solo cookies de sesión.

## Almacenamiento

- Genji v0.15.3 está fijado a la última versión bajo la ruta de módulo
  `genjidb/genji`; el desarrollo upstream se trasladó a Chai.
- El repositorio se mantiene dentro de un pequeño subconjunto de SQL de Genji:
  una expresión `ORDER BY`, sin `LOWER()`, sin upsert nativo.
- Sin framework de migración de esquema. Los cambios de esquema requieren una
  migración deliberada.
- Un proceso por directorio de datos; Genji mantiene un bloqueo.
- La base de datos y el registro de eventos no están cifrados en reposo.
- Sin programador de copias de seguridad integrado.

## Operaciones

- Sin imagen de contenedor ni empaquetado oficial más allá de `make install`.
- Sin clustering ni escalado horizontal de un solo conjunto de datos.
- Sin endpoint de métricas integrado (Prometheus u otro); usa `/healthz`,
  `/api/stats`, los registros y las métricas del host.
- Sin TLS integrado para el panel; usa un proxy inverso.
- Sin caché de Pebble configurable ni ajuste del almacenamiento.
- Sin soporte de proxy SMTP dentro del proceso.

## Frontend

- El binario incorpora el paquete completo de DaisyUI/Tailwind (~2.9 MiB), que
  domina el tamaño de la primera carga.
- No hay una canalización de compilación con Node; cambiar los recursos implica
  reemplazar archivos fijados y recompilar el binario.
- La página de eventos en vivo usa WebSocket; los navegadores con JavaScript
  deshabilitado ven las filas persistidas iniciales pero no actualizaciones en
  vivo.
- El panel no está localizado.

## Alcance del producto

- No es un MTA, un servidor IMAP/POP3, un proveedor de buzones ni un filtro de
  spam.
- Las políticas de descarte son agujeros negros silenciosos; los remitentes
  reciben éxito.
- Sin deduplicación de mensajes ni claves de idempotencia.
- Sin garantías de entrega más allá de la respuesta de la transacción SMTP.
