---
title: Autenticación
weight: 50
---

# Autenticación

MailVoid utiliza un modelo de administrador único: toda cuenta autenticada tiene
acceso completo a mensajes, políticas, eventos, ajustes y la API. No existen roles
ni permisos por buzón.

## Administrador de arranque inicial

La cuenta se crea en el primer inicio a partir de:

- `MAILVOID_ADMIN_USER` (por defecto `admin`; un valor explícitamente vacío falla
  la validación de configuración);
- `MAILVOID_ADMIN_PASSWORD`.

Si la contraseña está vacía, MailVoid genera una contraseña aleatoria de 24
caracteres segura para URL y la imprime una sola vez durante el arranque. Si la
cuenta ya existe, la contraseña se deja sin cambios salvo que
`MAILVOID_ADMIN_RESET_PASSWORD=true`.

```bash
# first start with a known password
MAILVOID_ADMIN_PASSWORD='change-me-please' ./mailvoid

# later, intentionally reset it
MAILVOID_ADMIN_PASSWORD='new-password' \
MAILVOID_ADMIN_RESET_PASSWORD=true \
./mailvoid
```

Las contraseñas se almacenan con hash bcrypt con coste 12. Las contraseñas en
texto plano nunca se guardan.

## Sesiones

- El inicio de sesión establece una cookie solo HTTP llamada `mailvoid_session`.
- Valores predeterminados de la cookie: `SameSite=Lax`, ruta `/`, caducidad de 12 horas.
- El ID de sesión se regenera tras un inicio de sesión correcto para prevenir la
  fijación de sesión.
- Las sesiones se mantienen en memoria. Reiniciar MailVoid cierra la sesión de todos.
- Establece `MAILVOID_COOKIE_SECURE=true` cuando el panel se sirve mediante HTTPS.

## Iniciar sesión

Abre `http://<host>:8025` y usa el formulario de inicio de sesión.

Para scripts, conserva un tarro de cookies:

```bash
curl -s -c cookies.txt \
  -d 'username=admin&password=change-me-please' \
  http://localhost:8025/login

curl -s -b cookies.txt http://localhost:8025/api/messages
```

Un inicio de sesión fallido devuelve HTTP `401` con la página de inicio de sesión
y emite `auth.login.failed` con el nombre de usuario intentado, la IP remota y el
user agent. La contraseña nunca se incluye en el evento.

## Cerrar sesión

El menú superior derecho envía una petición a `/logout`. La sesión se destruye y
se emite `auth.logout`.

## Cambiar la contraseña

**Ajustes → Contraseña del administrador** requiere la contraseña actual, una
contraseña nueva de al menos 8 caracteres y su confirmación. El éxito emite
`auth.password.changed` y redirige con un mensaje flash.

No existe ninguna política de fortaleza de contraseña más allá de la comprobación
de longitud. Usa una contraseña larga y única procedente de un gestor de secretos.

## Protección de rutas

| Ruta | Autenticación |
|-------|----------------|
| `/healthz` | Pública |
| `/static/*` | Pública |
| `GET/POST /login` | Pública |
| `/` y todas las páginas de la consola | Se requiere sesión |
| `/api/*` | Se requiere sesión |
| `/ws/events` | Se requiere sesión |
| `/logout` | Se requiere sesión |

Las peticiones de página no autenticadas redirigen a `/login?next=<original-url>`.
Las peticiones no autenticadas a la API y a WebSocket reciben HTTP `401` con
`{"error":"authentication required"}`.

Se valida que el parámetro `next` sea una ruta absoluta local; las redirecciones
externas se rechazan.

## Credenciales de agente

Para la automatización, configura `MAILVOID_API_TOKEN` y envía:

```http
Authorization: Bearer <token>
```

Esto se acepta en `/api/v1` y `/mcp`. No se acepta en las rutas heredadas `/api`
ni en la consola HTML. Si no se configura ningún token, las peticiones con bearer
se rechazan y los agentes deben usar una cookie de sesión.

Las claves de API gestionadas y los usuarios WebDAV también se pueden crear y
revocar en tiempo de ejecución a través de la API de administración. Consulta
[Credenciales gestionadas](/reference/credentials/).

## Credenciales de WebDAV

`/dav/` usa un par de autenticación Basic independiente:

- `MAILVOID_WEBDAV_USER` (por defecto `mailvoid`);
- `MAILVOID_WEBDAV_PASSWORD` (vacío deshabilita WebDAV).

Las credenciales de WebDAV no conceden acceso a la consola ni a la API. Consulta
[WebDAV](/operations/webdav/).

## Notas de seguridad

- Cualquier cuenta con sesión iniciada es administradora. Trata las credenciales
  de arranque inicial como credenciales de root del servicio.
- No se implementa limitación de tasa en el inicio de sesión. Coloca el panel
  detrás de un proxy inverso que pueda limitar la tasa, o restringe el acceso de red.
- No se implementan tokens CSRF. La cookie de sesión usa `SameSite=Lax`, lo que
  bloquea la mayoría de las peticiones POST de formularios entre sitios en los
  navegadores modernos. Añade un control a nivel de proxy si tu modelo de amenazas
  requiere defensa en profundidad.
- No hay interfaz de gestión de cuentas. Crea administradores adicionales solo a
  través de la API del repositorio si estás extendiendo el código; el flujo de
  arranque inicial incluido gestiona una única cuenta.
- Las cookies de sesión se marcan como `Secure` solo cuando se configura así. Usar
  `MAILVOID_COOKIE_SECURE=true` sin HTTPS hace que el inicio de sesión falle porque
  el navegador no devolverá la cookie.
