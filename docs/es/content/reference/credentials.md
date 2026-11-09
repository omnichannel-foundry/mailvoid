---
title: Credenciales gestionadas
weight: 22
---

# Credenciales gestionadas

MailVoid puede crear y revocar **claves de API** y **usuarios WebDAV** en tiempo de ejecución. Se almacenan en la base de datos integrada y surten efecto de inmediato; no se requiere reiniciar.

Las credenciales de arranque procedentes del entorno siguen siendo compatibles:

- `MAILVOID_API_TOKEN` — un token bearer de administrador.
- `MAILVOID_WEBDAV_USER` / `MAILVOID_WEBDAV_PASSWORD` — el par original de autenticación básica de WebDAV.

## Gestión exclusiva de administradores

Las rutas de gestión de credenciales están bajo `/api/v1/admin`. Requieren una de las siguientes opciones:

- una **sesión** de administrador autenticada (`POST /login`); o
- el token bearer `MAILVOID_API_TOKEN` de arranque.

Las claves de API gestionadas deliberadamente **no** pueden crear, listar ni eliminar credenciales. Esto evita la escalada de privilegios desde una clave de agente.

| Método | Ruta | Propósito |
|--------|------|-----------|
| `GET` | `/api/v1/admin/api-keys` | Listar claves gestionadas |
| `POST` | `/api/v1/admin/api-keys` | Crear una clave |
| `DELETE` | `/api/v1/admin/api-keys/{id}` | Revocar una clave |
| `GET` | `/api/v1/admin/webdav-users` | Listar usuarios WebDAV gestionados |
| `POST` | `/api/v1/admin/webdav-users` | Crear un usuario WebDAV |
| `DELETE` | `/api/v1/admin/webdav-users/{username}` | Eliminar un usuario WebDAV |

## Claves de API

Crea una clave con la sesión de administrador:

```bash
curl -s -c cookies.txt \
  -d 'username=admin&password=change-me' \
  http://localhost:8025/login

curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"name":"ci-agent","expires_at":"2027-01-01T00:00:00Z"}' \
  http://localhost:8025/api/v1/admin/api-keys
```

La respuesta contiene el **token en texto plano exactamente una vez**:

```json
{
  "id": "key_...",
  "name": "ci-agent",
  "prefix": "a1b2c3d4e5f6",
  "created_at": "2026-11-08T05:00:00Z",
  "expires_at": "2027-01-01T00:00:00Z",
  "token": "mv_a1b2c3d4e5f6_..."
}
```

Úsalo como `Authorization: Bearer mv_...` en `/api/v1` y `/mcp`. Solo se almacena un hash SHA-256 del token; el endpoint de listado devuelve el ID de la clave, el nombre, el prefijo, la fecha de creación, la fecha de expiración y el creador, nunca el token ni el hash.

- Los nombres son obligatorios.
- `expires_at` es opcional y debe estar en el futuro.
- Una clave gestionada tiene las mismas capacidades de agente que el token de arranque: leer correo, buscar, recibir eventos, usar herramientas MCP y eliminar mensajes. No puede gestionar credenciales.
- Revoca eliminando el ID de la clave. Las solicitudes que la usen reciben inmediatamente `401`.

## Usuarios WebDAV

```bash
# Generate a strong password (returned once)
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"username":"backup-agent"}' \
  http://localhost:8025/api/v1/admin/webdav-users

# Or supply a password of at least 12 characters
curl -s -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"username":"backup-agent","password":"a-long-password"}' \
  http://localhost:8025/api/v1/admin/webdav-users
```

Respuesta:

```json
{
  "username": "backup-agent",
  "password": "...",
  "generated": true,
  "created_at": "2026-11-08T05:00:00Z"
}
```

La contraseña se somete a hash con bcrypt y nunca se almacena en texto plano. Las comprobaciones exitosas de autenticación básica se almacenan en caché en memoria durante cinco minutos, de modo que las numerosas solicitudes de un cliente WebDAV no paguen el coste de bcrypt cada vez.

- Los nombres de usuario se convierten a minúsculas.
- Crear una credencial habilita la superficie `/dav/` incluso cuando `MAILVOID_WEBDAV_PASSWORD` está vacía.
- El nombre de usuario WebDAV de arranque está reservado mientras la contraseña de entorno esté establecida.
- Eliminar un usuario invalida las autenticaciones en caché y las solicitudes posteriores con esa credencial reciben `401`.

## Eventos

| Evento | Payload |
|--------|---------|
| `api_key.created` | `key_id`, `name`, `prefix`, `expires_at`, `username` |
| `api_key.deleted` | `key_id`, `name`, `prefix`, `username` |
| `webdav_user.created` | `username`, `generated`, `username_actor` |
| `webdav_user.deleted` | `username`, `username_actor` |

Los tokens y las contraseñas en texto plano nunca aparecen en los eventos.

## Seguridad

- Las claves de API gestionadas pueden leer y eliminar todo el correo capturado. Trátalas como secretos cercanos a los de administrador.
- Los usuarios WebDAV gestionados pueden leer todos los buzones a través de `/dav/`; no hay autorización por buzón.
- Usa HTTPS. La autenticación básica de WebDAV y los tokens bearer son visibles sobre HTTP sin cifrar.
- Rota creando una nueva credencial y eliminando la anterior.
- Guarda los tokens y las contraseñas generadas en un gestor de secretos; solo se muestran una vez.
