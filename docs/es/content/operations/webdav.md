---
title: WebDAV
weight: 45
---

# WebDAV

MailVoid expone un **árbol WebDAV de solo lectura** en `/dav/` para listar buzones
y descargar mensajes como archivos `.eml`.

## Habilitar

WebDAV se puede habilitar de dos formas:

1. Una contraseña de arranque (`MAILVOID_WEBDAV_PASSWORD`) configurada al arrancar.
2. Creando un usuario WebDAV gestionado a través de `/api/v1/admin/webdav-users`.

Los usuarios gestionados se crean, listan y revocan a través de la API de
administración; consulta [Credenciales gestionadas](/reference/credentials/). Las
comprobaciones de autenticación Basic correctas se almacenan en caché durante
cinco minutos.

Credenciales de arranque:

```bash
MAILVOID_WEBDAV_USER=mailvoid \
MAILVOID_WEBDAV_PASSWORD='replace-with-a-strong-password' \
./mailvoid
```

| Variable | Predeterminado | Significado |
|----------|---------|---------|
| `MAILVOID_WEBDAV_USER` | `mailvoid` | Nombre de usuario para autenticación Basic |
| `MAILVOID_WEBDAV_PASSWORD` | *(vacío)* | Contraseña para autenticación Basic; vacía deshabilita WebDAV |

Flags equivalentes: `-webdav-user`, `-webdav-password`.

Cuando no existe ni una contraseña de arranque ni ningún usuario gestionado,
`/dav/` devuelve `404`. Cuando está habilitado, las solicitudes no autenticadas
reciben `401` con un desafío `WWW-Authenticate: Basic`.

> [!WARNING]
> La autenticación Basic sobre HTTP sin cifrar expone la contraseña en la red.
> Sirve MailVoid a través de un proxy inverso HTTPS (y establece
> `MAILVOID_COOKIE_SECURE=true` para la consola). WebDAV no usa la sesión de
> administrador ni el token de API; tiene sus propias credenciales.

## Estructura de URL

```text
/dav/                                  mailbox directories
/dav/inbox@example.test/               messages in that mailbox
/dav/inbox@example.test/msg_abc.eml    one raw RFC 5322 message
```

- Los nombres de directorio son `mailbox@domain`.
- Los nombres de archivo son `<message-id>.eml`.
- Solo se listan los buzones con al menos un mensaje capturado.
- Los archivos son los bytes exactos almacenados por MailVoid, con `Content-Type:
  message/rfc822`.

## Métodos permitidos

| Método | Comportamiento |
|--------|----------|
| `OPTIONS` | Capacidades |
| `PROPFIND` | Lista directorios y archivos |
| `GET`, `HEAD` | Descarga un mensaje o inspecciona metadatos |

Todo lo demás devuelve `405 Method Not Allowed` con una cabecera `Allow`. No es un
destino de copia de seguridad: se rechazan `PUT`, `DELETE`, `MKCOL`, `MOVE`,
`COPY`, `LOCK`, `PROPPATCH` y `UNLOCK`.

## Ejemplos

### Listar buzones

```bash
curl -s -u 'mailvoid:replace-with-a-strong-password' \
  -X PROPFIND -H 'Depth: 1' \
  --data '<?xml version="1.0"?><propfind xmlns="DAV:"><allprop/></propfind>' \
  http://localhost:8025/dav/
```

La respuesta es XML `207 Multi-Status` con los directorios de buzones.

### Listar los mensajes de un buzón

```bash
curl -s -u 'mailvoid:replace-with-a-strong-password' \
  -X PROPFIND -H 'Depth: 1' \
  --data '<?xml version="1.0"?><propfind xmlns="DAV:"><allprop/></propfind>' \
  'http://localhost:8025/dav/inbox@example.test/'
```

### Descargar un mensaje

```bash
curl -u 'mailvoid:replace-with-a-strong-password' \
  -o message.eml \
  'http://localhost:8025/dav/inbox@example.test/msg_81997d51ccdda1a5.eml'
```

### Montar con davfs2

Los montajes de solo lectura funcionan con un cliente WebDAV estándar:

```bash
sudo mount -t davfs -o ro \
  http://localhost:8025/dav/ /mnt/mailvoid
```

Proporciona el nombre de usuario y la contraseña de WebDAV cuando se soliciten.
Como el árbol es de solo lectura, las escrituras fallan tanto en el cliente como en
el servidor.

## Relación con la consola y la API

| Superficie | Uso |
|---------|-----|
| Consola | Inspección humana; botón **Download .eml** en un mensaje |
| `/api/v1/messages/{id}/eml` | Descarga de `.eml` mediante script con autenticación bearer o de sesión |
| `/dav/` | Clientes WebDAV y montajes de gestores de archivos |

Los tres devuelven el mismo mensaje en bruto almacenado.

## Límites

- La autenticación de WebDAV es una credencial global separada, no por usuario.
- La agregación de directorios y el listado de mensajes recorren el almacén
  incrustado; las instalaciones muy grandes hacen que `PROPFIND` sea más lento.
- No hay soporte de escritura, movimiento, bloqueo ni mutación de propiedades.
- No hay autorización por mensaje; cualquiera con las credenciales de WebDAV puede
  leer todos los buzones.
- Usa HTTPS. La autenticación Basic no está cifrada por sí misma.
