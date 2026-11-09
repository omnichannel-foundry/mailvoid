---
title: CLI y flags
weight: 10
---

# CLI y flags

Sin subcomando, MailVoid inicia los listeners SMTP, el servidor HTTP, el proceso de barrido y los destinos de eventos en un solo proceso. También ofrece un pequeño comando offline `users` para el mantenimiento de cuentas administradoras.

```text
mailvoid [flags]
mailvoid users list [flags]
mailvoid users passwd [username] [flags]
mailvoid --version
```

`--version` imprime la versión inyectada, el commit y la fecha de compilación, y luego sale antes de cargar la configuración.

## Usuarios administradores

Los comandos `users` abren directamente la base de datos configurada. Detén primero el servidor MailVoid: la base de datos toma un bloqueo exclusivo, por lo que un servidor en ejecución bloquea estos comandos.

```bash
# Lista las cuentas administradoras.
mailvoid users list

# Cambia interactivamente la contraseña del administrador configurado.
mailvoid users passwd

# Cambia una cuenta concreta, opcionalmente con otro archivo de configuración.
mailvoid users passwd admin -config /etc/mailvoid/mailvoid.toml

# Uso no interactivo: lee la contraseña y su confirmación desde stdin.
printf 'new-password\nnew-password\n' | mailvoid users passwd admin
```

Si se omite `username`, se usa el nombre de administrador configurado. Las contraseñas deben tener al menos 8 caracteres; los prompts se ocultan cuando stdin es un terminal.

## Flags

| Flag | Tipo | Valor predeterminado | Descripción |
|------|------|----------------------|-------------|
| `-config` | string | `./mailvoid.toml` | Archivo de configuración |
| `-hostname` | string | `localhost` | Hostname público para enlaces absolutos |
| `-scheme` | string | `http` | Esquema de URL público (`http`/`https`) |
| `-public-url` | string | *(vacío)* | Anulación de la URL base pública absoluta |
| `-smtp-host` | string | `0.0.0.0` | Host de escucha SMTP único |
| `-smtp-hosts` | string | *(vacío)* | Hosts de escucha SMTP separados por comas |
| `-smtp-ports` | string | `1025,2525` | Puertos separados por comas/espacios |
| `-http-addr` | string | `:8025` | Dirección de escucha HTTP |
| `-web-bind` | string | *(desde http-addr)* | Interfaz de escucha HTTP |
| `-web-port` | int | *(desde http-addr)* | Puerto de escucha HTTP |
| `-domains` | string | `*` | Dominios aceptados |
| `-data-dir` | string | `./data` | Directorio de datos |
| `-db-path` | string | `<data-dir>/mailvoid.db` | Directorio de la base de datos Genji |
| `-event-log` | string | `<data-dir>/events.jsonl` | Registro de eventos JSONL |
| `-event-log-enabled` | bool | `true` | Habilita el registro de eventos JSONL |
| `-log-enabled` | bool | `true` | Habilita los logs tradicionales por servicio |
| `-log-dir` | string | `<data-dir>/logs` | Directorio para los logs de servicio |
| `-admin-user` | string | `admin` | Nombre de usuario de arranque |
| `-admin-password` | string | *(vacío)* | Contraseña de arranque |
| `-admin-reset-password` | bool | `false` | Restablece la contraseña de un administrador existente |
| `-sweep-interval` | duration | `30s` | Intervalo de barrido de TTL |
| `-max-message-bytes` | int64 | `26214400` | Tamaño máximo del mensaje |
| `-read-timeout` | duration | `60s` | Tiempo de espera de lectura/escritura SMTP |
| `-cookie-secure` | bool | `false` | Cookie de sesión segura |
| `-api-token` | string | *(vacío)* | Token bearer para `/api/v1` y `/mcp` |
| `-webdav-user` | string | `mailvoid` | Nombre de usuario de autenticación básica WebDAV |
| `-webdav-password` | string | *(vacío)* | Contraseña de arranque WebDAV; vacía deshabilita la credencial de arranque |
| `-search-enabled` | bool | `true` | Habilita la búsqueda de texto completo de Bleve |
| `-search-index` | string | `<data-dir>/search.bleve` | Directorio del índice de búsqueda |

El paquete flag de Go acepta tanto `-flag` como `--flag`. Los flags booleanos aceptan `-cookie-secure`, `-cookie-secure=true` y `-cookie-secure=false`.

## Entorno

Cada flag tiene un equivalente en la variable de entorno `MAILVOID_`. Los flags anulan los valores de entorno. Consulta [Configuración](/getting-started/configuration/) para la tabla completa, la resolución de rutas y las reglas de validación.

## Comportamiento de salida

| Situación | Resultado |
|-----------|-----------|
| `--version` | Imprime metadatos, sale con 0 |
| Configuración inválida | Registra `configuration: ...`, sale con 1 |
| Fallo al abrir la base de datos/registro de eventos | Registra el error, sale con 1 |
| Fallo al enlazar el listener SMTP | Registra el error, sale con 1 |
| Fallo del listener HTTP | Registra el error, se apaga de forma limpia, sale con 1 |
| `SIGINT` / `SIGTERM` | Apagado ordenado, sale con 0 |

## Señales

`SIGINT` y `SIGTERM` desencadenan:

1. Dejar de aceptar trabajo nuevo (cancelación del contexto).
2. Detener el barrido de TTL.
3. Emitir `system.shutdown`.
4. Cerrar las conexiones HTTP, SSE y WebSocket.
5. Cerrar los listeners SMTP y las conexiones activas.
6. Vaciar y cerrar el registro de eventos JSONL.
7. Cerrar la base de datos Genji.

## Targets de Make

| Target | Descripción |
|--------|-------------|
| `make build` | Compila `bin/mailvoid` con metadatos de versión |
| `make release` | Binario de release estático y sin símbolos (stripped) |
| `make run` | Compila y ejecuta (`ARGS='...'` para flags) |
| `make test` | Ejecuta todas las pruebas |
| `make test-race` | Detector de carreras, sin caché |
| `make cover` | Informe de cobertura HTML |
| `make vet` | `go vet` |
| `make fmt` | gofumpt si está instalado; de lo contrario, gofmt |
| `make fmt-check` | Falla si el formato difiere |
| `make lint` | golangci-lint (debe estar instalado) |
| `make check` | fmt-check + vet + test |
| `make tidy`, `make tidy-check` | Limpieza de módulos |
| `make it` | Compila el ejecutor de integración independiente |
| `make integration` | Ejecuta la suite JTD en modo por lotes |
| `make integration-tui` | Ejecuta la suite JTD en la TUI interactiva |
| `make install`, `make uninstall` | Instala/elimina bajo `PREFIX/bin` |
| `make clean` | Elimina `bin/` y la salida de cobertura |
