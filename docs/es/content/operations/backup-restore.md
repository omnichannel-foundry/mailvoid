---
title: Copia de seguridad y restauración
weight: 40
---

# Copia de seguridad y restauración

MailVoid tiene dos piezas de estado:

1. `<data-dir>/mailvoid.db/` — el directorio de base de datos Genji/Pebble.
2. `<data-dir>/events.jsonl` — el registro de eventos JSONL de solo anexado.
3. `<data-dir>/search.bleve/` — el índice de texto completo de Bleve.

El índice de búsqueda se puede reconstruir desde Genji; puedes excluirlo para
ahorrar espacio y tiempo de copia de seguridad. Contiene términos de mensajes
indexados para búsqueda, así que si haces copia de él, aplica los mismos
controles de cifrado y acceso que a la base de datos.

## Copia de seguridad segura

Pebble utiliza un registro de escritura anticipada (write-ahead log). No copies
el directorio de la base de datos mientras el proceso está escribiendo y esperes
obtener una instantánea válida. Elige una opción:

### Detener, copiar, iniciar

```bash
sudo systemctl stop mailvoid
sudo tar -C /var/lib -czf /backup/mailvoid-$(date +%F).tgz mailvoid
sudo systemctl start mailvoid
```

### Instantánea del sistema de archivos

Si tu sistema de archivos admite instantáneas atómicas (LVM, ZFS, btrfs), toma
una instantánea del volumen mientras el proceso se está ejecutando. La
instantánea es consistente aunque el directorio activo no sea seguro de copiar
directamente.

### Exportación vía API

Para una exportación lógica sin detener el servicio:

```bash
curl -s -b cookies.txt 'http://127.0.0.1:8025/api/messages?limit=200' > messages.json
```

Esto solo devuelve metadatos paginados; usa el endpoint de detalle por mensaje si
necesitas el contenido en bruto. No es una copia de seguridad de fidelidad total
de políticas, usuarios ni eventos.

## Restauración

```bash
sudo systemctl stop mailvoid
sudo mv /var/lib/mailvoid /var/lib/mailvoid.old
sudo tar -C /var/lib -xzf /backup/mailvoid-2026-11-08.tgz
sudo chown -R mailvoid:mailvoid /var/lib/mailvoid
sudo chmod 0750 /var/lib/mailvoid
sudo systemctl start mailvoid
```

Verifica:

```bash
curl -s http://127.0.0.1:8025/healthz
curl -s -b cookies.txt http://127.0.0.1:8025/api/stats
```

Si la contraseña del administrador o el secreto de sesión cambian, las sesiones
de navegador existentes dejan de ser válidas; vuelve a iniciar sesión.

## Rotación del registro de eventos

El archivo JSONL es de solo anexado e independiente de la base de datos. Cuando
el servicio está detenido, puedes archivarlo o truncarlo sin afectar a los
mensajes ni a las políticas:

```bash
sudo systemctl stop mailvoid
sudo mv /var/lib/mailvoid/events.jsonl /backup/events-$(date +%F).jsonl
sudo systemctl start mailvoid
```

La tabla `events` de Genji conserva su propio historial. Eliminar el archivo
JSONL no elimina los eventos de la API ni de la consola.

## Qué no se incluye en la copia de seguridad

- El almacén de sesiones está en memoria; las sesiones son efímeras a propósito.
- No existen almacenamiento de objetos externo, cola de correo ni cachés.
- Los artefactos de compilación y los recursos incrustados son reproducibles a
  partir del binario.

## Validación

Después de una restauración, comprueba:

- que la base de datos se abre sin un error de bloqueo;
- que el recuento de mensajes coincide con lo esperado;
- que las políticas aparecen en el orden correcto;
- que los eventos están presentes en la consola y en `events.jsonl`;
- que los mensajes con TTL tienen tiempos de caducidad razonables (las marcas de
  tiempo absolutas se conservan, así que restaurar una copia antigua puede hacer
  que los mensajes caduquen de inmediato).
