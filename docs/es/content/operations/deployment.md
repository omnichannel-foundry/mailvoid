---
title: Despliegue
weight: 10
---

# Despliegue

## Binario y directorio de datos

Despliega el binario de la versión y elige un directorio de estado con permisos
de escritura:

```bash
sudo install -D -m 0755 bin/mailvoid /usr/local/bin/mailvoid
sudo install -d -o mailvoid -g mailvoid /var/lib/mailvoid
```

Ejemplo de archivo de entorno (`/etc/mailvoid/mailvoid.env`, modo `0600`):

```bash
MAILVOID_SMTP_HOST=0.0.0.0
MAILVOID_SMTP_PORTS=1025,2525
MAILVOID_HTTP_ADDR=127.0.0.1:8025
MAILVOID_DOMAINS=app.test,*.mail.test
MAILVOID_DATA_DIR=/var/lib/mailvoid
MAILVOID_ADMIN_USER=admin
MAILVOID_ADMIN_PASSWORD=change-me-from-a-secret-manager
MAILVOID_COOKIE_SECURE=true
# Optional agent and WebDAV surfaces:
# MAILVOID_API_TOKEN=replace-with-a-long-random-token
# MAILVOID_WEBDAV_USER=mailvoid
# MAILVOID_WEBDAV_PASSWORD=replace-with-another-secret
```

Inícialo bajo un supervisor de procesos. La [unidad de systemd](/operations/systemd/)
es un ejemplo completo.

## Distribución de red

Despliegue típico:

```text
SMTP clients ── 1025/2525 ──▶ MailVoid ── 127.0.0.1:8025 ──▶ TLS reverse proxy ──▶ browser
```

- Vincula HTTP a la interfaz de loopback y deja que un proxy inverso termine TLS.
- Expón solo los puertos SMTP que necesites.
- Restringe SMTP con reglas de cortafuegos o una red privada. MailVoid no
  autentica a los clientes SMTP.
- Establece `MAILVOID_COOKIE_SECURE=true` cuando se use HTTPS de extremo a
  extremo.

## Puertos por debajo de 1024

Los valores predeterminados (`1025`, `2525`) no requieren privilegios. Para
vincular el puerto `25` o `587`, concede la capacidad en lugar de ejecutar como
root:

```ini
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
```

## Reloj

Las marcas de tiempo de eventos y mensajes provienen del reloj del host. Ejecuta
NTP o `systemd-timesyncd`. Todas las horas persistidas están en UTC; la consola
renderiza en UTC mientras que el cliente de eventos en vivo muestra la hora
local.

## Disco

Planifica para:

- mensajes en bruto, cuerpos decodificados y cabeceras en la base de datos Genji
  (una copia por cada destinatario del sobre);
- la tabla de eventos de solo anexado y el registro JSONL;
- el registro de escritura anticipada de la base de datos y el margen para la
  compactación.

No hay cuota ni retención integradas para los eventos. Supervisa el espacio libre
y configura una alerta externa.

## Instancia única

Genji mantiene un `LOCK` sobre su directorio. No ejecutes dos procesos de
MailVoid contra el mismo directorio de datos. Para escalar horizontalmente, da a
cada instancia su propio directorio de datos, puertos y dirección HTTP, y enruta
los clientes de forma explícita.

## Proxy inverso y TLS

El panel de control y la API deben servirse por HTTPS. Consulta
[Proxy inverso](/operations/reverse-proxy/) para ejemplos con Nginx y Caddy,
incluidas las configuraciones de WebSocket y SSE.

> [!WARNING]
> HTTPS en el panel de control no cifra el tráfico SMTP. MailVoid no tiene STARTTLS.
> Envía correo a través de una red de confianza o un túnel.

## Nota sobre contenedores

No hay una imagen de contenedor oficial en este repositorio. Un contenedor
construido a partir del binario de la versión funciona si:

- montas `/var/lib/mailvoid` para la persistencia;
- publicas los puertos SMTP y HTTP;
- estableces las variables de entorno anteriores;
- mantienes el directorio de datos en una única instancia.

Como la imagen no se distribuye, sigue la política de imagen base y cadena de
suministro de tu organización.

## Comprobaciones de estado

Usa `GET /healthz` para la comprobación de actividad. Es público, barato y no
toca la base de datos:

```bash
curl -fsS http://127.0.0.1:8025/healthz
```

Una comprobación de disponibilidad más profunda es `GET /api/stats` con una
sesión válida; consulta mensajes, políticas y eventos.
