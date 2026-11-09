---
title: Seguridad
weight: 50
---

# Seguridad

MailVoid es una herramienta administrativa que almacena mensajes entrantes
completos, incluidas cabeceras y cuerpos. Trata el directorio de datos y el panel
de control como información sensible.

## Modelo de amenazas

| Activo | Riesgo |
|-------|------|
| Mensajes almacenados | Contienen PII, credenciales en enlaces de restablecimiento, adjuntos, cabeceras internas |
| Mensajes en bruto | Pueden incluir malware o píxeles de seguimiento |
| Registro de eventos | Contiene asuntos, remitentes, destinatarios, IP, nombres de usuario |
| Credenciales de administrador | Acceso total a todo el correo, políticas y ajustes |
| Listener SMTP | Sin autenticación; cualquiera que pueda alcanzarlo puede enviar correo |
| Directorio de la base de datos | Sin cifrar en reposo |

## Exposición de red

- **Restringe SMTP.** MailVoid no autentica a los clientes SMTP. Un listener
  público con `MAILVOID_DOMAINS=*` es un sumidero abierto: cualquiera puede llenar
  el disco.
- **Vincula HTTP a loopback** y termina TLS en un proxy inverso.
- **Protege con cortafuegos los puertos de gestión.** No expongas el panel de
  control ni la API a internet sin autenticación y TLS.
- **Prefiere una red privada** (VPN, WireGuard, VPC privada) para SMTP entre los
  hosts de aplicación y MailVoid.

## SMTP no tiene TLS ni autenticación

No hay `STARTTLS`, ni listener TLS implícito, ni `AUTH`. Las credenciales y el
contenido de los mensajes viajan en texto claro en el salto SMTP. Esta es una
restricción minimalista deliberada, no una opción de configuración.

Si se requiere confidencialidad SMTP, haz un túnel para el salto a través de una
red privada o coloca una pasarela que entienda SMTP delante de MailVoid.

## Autenticación del panel de control

- Las contraseñas son hashes bcrypt con coste 12.
- La cookie de sesión es HTTP-only y `SameSite=Lax`; caduca a las 12 horas.
- Establece `MAILVOID_COOKIE_SECURE=true` detrás de HTTPS.
- Toda cuenta autenticada es administradora. No hay un modelo de roles de mínimo
  privilegio.
- No se implementan límites de tasa de inicio de sesión ni tokens CSRF. Usa
  restricciones de red o un proxy que pueda añadir limitación de tasa y
  protección CSRF.
- La contraseña generada en la primera ejecución se imprime en el registro del
  proceso. Protege el acceso al journal y cambia la contraseña después de iniciar
  sesión.

## Secretos

No confirmes en el repositorio `MAILVOID_ADMIN_PASSWORD`, `MAILVOID_API_TOKEN` ni
`MAILVOID_WEBDAV_PASSWORD`. Usa un gestor de secretos o un `EnvironmentFile` de
systemd con modo `0600`. Los flags que llevan secretos son visibles en la lista de
procesos; prefiere variables de entorno o un archivo de entorno.

- `MAILVOID_API_TOKEN` concede acceso de lectura y eliminación a todos los
  mensajes a través de `/api/v1` y `/mcp`. Rótalo cambiando el valor y
  reiniciando.
- `MAILVOID_WEBDAV_PASSWORD` concede acceso de lectura a todos los buzones a
  través de `/dav/`. Es independiente de la cuenta de administrador y del token
  de API.
- WebDAV usa autenticación HTTP Basic. Sin TLS, la contraseña es visible en la
  red; termina siempre HTTPS delante de él.

## Datos en reposo

La base de datos Genji y el registro de eventos JSONL no están cifrados. Usa
cifrado de disco completo o de volumen, permisos de archivo restrictivos
(directorio `0750`) y copias de seguridad cifradas en tránsito y en reposo.

Permisos de archivo:

```bash
chown -R mailvoid:mailvoid /var/lib/mailvoid
chmod 0750 /var/lib/mailvoid
chmod 0600 /var/lib/mailvoid/events.jsonl
```

## Seguridad del contenido de correo

- Los cuerpos HTML se renderizan en un iframe aislado (sandbox) sin scripts ni
  acceso al mismo origen.
- El texto plano y los mensajes en bruto se escapan como HTML antes de mostrarse.
- Las imágenes y estilos remotos dentro del HTML aislado aún pueden realizar
  solicitudes de red. Si eso es una preocupación, inspecciona el mensaje en bruto
  en su lugar.
- Los adjuntos se almacenan dentro del mensaje en bruto. MailVoid no analiza en
  busca de malware. No abras adjuntos extraídos en un host de confianza.
- Las cabeceras de correo pueden contener remitentes falsificados; `sender` es el
  sobre SMTP, no una identidad criptográfica.

## Sensibilidad del registro de auditoría

Los eventos incluyen asuntos, direcciones de remitente y destinatario, IP de
origen, nombres de usuario y patrones de políticas. Pueden contener datos
personales. Aplica los mismos controles de acceso y política de retención a
`events.jsonl` y a la tabla `events` de Genji que a los propios mensajes.

El historial de eventos no tiene límite. Define un proceso externo de
retención/archivado si las restricciones normativas o de disco lo requieren.

## Sensibilidad del índice de búsqueda

El índice de Bleve contiene términos de mensajes analizados y campos de búsqueda
almacenados. Trata `search.bleve/` como algo tan sensible como la base de datos
Genji. Los controles de acceso, las copias de seguridad y el cifrado de disco
deben cubrirlo. Es reconstruible desde Genji, así que puede omitirse de las copias
de seguridad cuando el tiempo de reconstrucción sea aceptable.

## Riesgos de credenciales gestionadas

- Las claves de API y los usuarios de WebDAV se gestionan a través de
  `/api/v1/admin`, que requiere una sesión de administrador o el token de API de
  arranque. Las claves de API gestionadas no pueden gestionar credenciales.
- Las claves de API gestionadas pueden leer y eliminar todo el correo capturado.
  Trátalas como secretos adyacentes al administrador y rótalas creando un
  reemplazo y eliminando la clave antigua.
- Los usuarios de WebDAV gestionados pueden leer todos los buzones y no están
  limitados por buzón.
- Los tokens/contraseñas generados se devuelven una sola vez. Guárdalos en un
  gestor de secretos.

## Riesgos del agente y de la superficie de archivos

- MCP incluye `delete_message`. Un agente con el token de API puede destruir de
  forma permanente el correo capturado.
- `/api/v1`, `/mcp` y la consola exponen el contenido en bruto de los mensajes;
  trata el token de API como una credencial de administrador.
- WebDAV expone el mismo contenido en bruto a cualquiera que tenga el par de
  autenticación Basic y no tiene autorización por buzón.
- Las descargas `.eml` son los bytes exactos almacenados, incluidos adjuntos y
  contenido activo. No los abras en un host de confianza sin analizarlos.
- WebDAV y MCP no tienen estado; no hay revocaciones por sesión. Rota las
  credenciales y reinicia para invalidar las antiguas.

## Denegación de servicio

- `MAILVOID_MAX_MESSAGE_BYTES` limita un único mensaje (25 MiB por defecto).
- No hay límite de tasa ni de conexiones por cliente.
- Cada mensaje se almacena una vez por destinatario.
- La persistencia de eventos añade amplificación de escritura por cada transacción
  SMTP.
- La tabla de eventos y el archivo JSONL crecen sin compactación.

Para remitentes no confiables, restringe los dominios, protege los puertos SMTP
con cortafuegos, monitoriza el disco y establece un límite de tamaño de mensaje
adecuado a la carga de trabajo.

## Riesgos de dependencias y runtime

- Genji v0.15.3 está fijado porque las versiones más recientes renombraron el
  módulo a Chai y requieren una cadena de herramientas de Go más nueva. Sigue los
  avisos de seguridad upstream y planifica un reemplazo del repositorio o una
  migración si es necesario.
- Los recursos del frontend incrustados (DaisyUI, HTMX, Cash) están fijados en el
  repositorio. Revisa las actualizaciones de forma deliberada.
- El binario está enlazado estáticamente; mantén la cadena de herramientas de
  compilación parcheada.

## Lista de verificación de endurecimiento

- [ ] Puertos SMTP accesibles solo desde redes de confianza
- [ ] `MAILVOID_DOMAINS` restringido a los dominios que realmente usas
- [ ] HTTP vinculado a loopback
- [ ] Proxy inverso TLS delante del panel de control
- [ ] `MAILVOID_COOKIE_SECURE=true`
- [ ] Contraseña de administrador larga y única desde un gestor de secretos
- [ ] Permisos del directorio de datos `0750`, archivos `0600`
- [ ] Cifrado de disco para el volumen de datos
- [ ] Acceso a registros y eventos restringido
- [ ] Proceso de copia de seguridad probado
- [ ] Monitorización de espacio en disco y estado del servicio
- [ ] Límite de tamaño de mensaje revisado
- [ ] Proceso externo de retención/archivado de eventos definido
- [ ] `MAILVOID_API_TOKEN` establecido, guardado en un gestor de secretos y rotado periódicamente
- [ ] WebDAV deshabilitado, o `MAILVOID_WEBDAV_PASSWORD` establecido y servido solo por HTTPS
- [ ] Clientes MCP de confianza con el token de API y acceso destructivo a `delete_message`
- [ ] Directorio del índice de búsqueda cubierto por los mismos controles de acceso y cifrado que la base de datos
