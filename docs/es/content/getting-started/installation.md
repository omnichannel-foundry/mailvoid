---
title: Instalación
weight: 10
---

# Instalación

## Requisitos previos

- **Go 1.24.2 o posterior** para compilar desde el código fuente.
- **GNU Make** (opcional, recomendado).
- **Hugo 0.158 o posterior** solo si quieres compilar este sitio de
  documentación.
- No se requiere CGO, Node.js, npm ni una base de datos externa para la
  aplicación.

El módulo fija dependencias que compilan en Go 1.24.2:

- `github.com/genjidb/genji v0.15.3`
- `github.com/gofiber/fiber/v2 v2.52.16`
- `github.com/emersion/go-smtp v0.25.1`
- `github.com/gofiber/websocket/v2 v2.2.1`
- `github.com/blevesearch/bleve/v2 v2.5.7`
- `golang.org/x/crypto v0.42.0`
- `golang.org/x/net v0.45.0`
- `github.com/modelcontextprotocol/go-sdk v1.4.0`

## Compilar con Make

```bash
git clone <your-repository-url>
cd mailvoid
make build
./bin/mailvoid --version
```

`make build` inyecta la versión, el commit y la fecha de compilación mediante
`-ldflags` y escribe `bin/mailvoid`.

Para un binario de release eliminado y enlazado estáticamente:

```bash
make release
```

Esto establece `CGO_ENABLED=0` y añade `-s -w`, produciendo un binario más
pequeño.

## Compilar sin Make

```bash
go build -trimpath -o mailvoid ./cmd/mailvoid
```

## Instalar

```bash
make install                            # ~/.local/bin/mailvoid
sudo make install PREFIX=/usr/local     # /usr/local/bin/mailvoid
```

La ubicación de instalación predeterminada es `$(PREFIX)/bin/mailvoid` con
`PREFIX="$HOME/.local"`. Usa `DESTDIR` para empaquetado por etapas.

## Primera ejecución

```bash
MAILVOID_ADMIN_PASSWORD='choose-a-strong-password' ./bin/mailvoid
```

MailVoid crea `./data`, abre `./data/mailvoid.db` y anexa a
`./data/events.jsonl`. Luego registra los listeners SMTP y la dirección del
panel:

```text
mailvoid created administrator "admin" from configured credentials
mailvoid SMTP listening on [::]:1025
mailvoid SMTP listening on [::]:2525
mailvoid HTTP dashboard listening on :8025
```

Si omites `MAILVOID_ADMIN_PASSWORD`, se genera una contraseña aleatoria en el
primer arranque y se imprime en el log exactamente una vez:

```text
mailvoid created administrator "admin" with generated password: ...
```

Guarda esa contraseña y cámbiala en **Ajustes** después de iniciar sesión.

## Verificar

```bash
curl -s http://localhost:8025/healthz
# {"status":"ok","version":"...","uptime":"..."}
```

## Actualizar

1. Detén el proceso antiguo.
2. Reemplaza el binario.
3. Inícialo con el mismo directorio de datos.

El esquema se crea con `CREATE TABLE IF NOT EXISTS`. No hay un marco general de
migración de esquema; añadir nuevas columnas a una base de datos existente
requiere un paso de migración deliberado. Haz una copia de seguridad del
directorio de datos antes de actualizar entre versiones que cambien el esquema.

## Desinstalar

```bash
make uninstall
```

Elimina solo el binario; el Makefile nunca borra el directorio de datos. Haz una
copia de seguridad primero si contiene mensajes que necesitas.
