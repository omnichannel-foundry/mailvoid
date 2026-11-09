---
title: Pruebas
weight: 20
---

# Pruebas

## Comandos

```bash
make test        # all packages
make test-race   # race detector, uncached
make cover       # coverage.out + HTML report
make check       # fmt-check + vet + test
```

Los comandos habituales de Go también funcionan:

```bash
go test ./...
go test -race -count=1 ./internal/storage
go test -run TestPolicy ./internal/policy -v
```

## Capas de pruebas

| Paquete | Qué se cubre |
|---------|--------------|
| `internal/events` | Distribución del bus, anillo de historial, baja de suscripción, destino JSONL |
| `internal/storage` | Suite compartida ejecutada contra memoria y Genji: ida y vuelta, filtros, paginación, caducidad, orden de políticas, eventos, usuarios |
| `internal/policy` | Gana la primera coincidencia, todos los campos, caducidades TTL, reglas deshabilitadas/inválidas, validación, probador de regex |
| `internal/mailparse` | Texto plano, asuntos codificados, multipart/base64, quoted-printable/Latin-1, solo HTML, adjuntos, entrada malformada |
| `internal/auth` | Hash/verificación, búsqueda de usuario sin distinguir mayúsculas, idempotencia de la inicialización, restablecimiento, contraseñas generadas |
| `internal/config` | Valores por defecto, precedencia de entorno/flags, análisis de puertos, comodines de dominio, valores inválidos, rutas personalizadas |
| `internal/smtpserver` | Cliente SMTP real contra listeners efímeros: captura, múltiples puertos, rechazo de dominio, políticas de descarte/TTL, múltiples destinatarios, mensajes no analizables, correo sobredimensionado, fallos de almacenamiento/política, errores de sobre |
| `internal/mailbox` | Agregación de buzones, paginación, resúmenes, filtros de mensajes, esperas de entrega |
| `internal/web` | Controles de autenticación, tokens bearer, inicio/cierre de sesión, cambio de contraseña, panel, fragmentos HTMX, API heredada y v1, endpoints de buzón/espera/`.eml`, políticas, eventos, estadísticas, 404, SSE, WebSocket, herramientas MCP, sistema de archivos WebDAV de solo lectura, servicio de OpenAPI |

## Estrategia de pruebas del almacenamiento

`internal/storage/store_test.go` define una suite y la ejecuta dos veces:

```go
for name, store := range stores(t) {
    t.Run(name, func(t *testing.T) {
        // shared behavior assertions
    })
}
```

Cualquier nuevo método de repositorio debe ejercitarse contra ambas
implementaciones. Esta es la red de seguridad para el subconjunto SQL de Genji.

## Pruebas de integración SMTP

Las pruebas enlazan el puerto `0` para que el sistema operativo asigne uno libre y
luego leen la dirección real de `Server.Addrs()`. Usan `net/smtp.SendMail` para
ejercitar la ruta completa del protocolo y comprueban los mensajes almacenados y
los eventos emitidos.

```go
srv, store, bus := startServer(t, testConfig(0))
addr := srv.Addrs()[0].String()
err := smtp.SendMail(addr, nil, "sender@example.org",
    []string{"inbox@example.test"}, []byte(sampleMessage))
```

## Pruebas web

`app.Test` de Fiber ejecuta las peticiones en proceso. Un pequeño helper de
cookies realiza un inicio de sesión real y reutiliza la cookie de sesión:

```go
resp := request(t, srv, http.MethodPost, "/login",
    strings.NewReader(form.Encode()), nil,
    map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
cookie := findSessionCookie(resp)
```

Para las pruebas de WebSocket, inicia la aplicación en un listener real y conéctate
con `github.com/fasthttp/websocket`:

```go
ln, _ := net.Listen("tcp", "127.0.0.1:0")
go srv.App().Listener(ln)
conn, _, _ := websocket.DefaultDialer.Dial("ws://"+ln.Addr()+"/ws/events", headers)
```

## Cobertura

Los paquetes de la aplicación cubren aproximadamente el 89% de las sentencias; el
total, incluido el ejecutor de integración independiente, es de alrededor del 84%.
La cobertura es una guía, no un objetivo: los casos importantes aquí son los
fallos de sobre, la precedencia de políticas, la paridad de almacenamiento entre
memoria y Genji, los límites de sesión/bearer, las credenciales gestionadas, la
búsqueda/alternativa de Bleve, la aplicación de solo lectura de WebDAV y los
resultados de las herramientas MCP.

## Detector de carreras

La suite completa de carreras forma parte de la verificación esperada para los
cambios en almacenamiento, eventos, SMTP o código web:

```bash
make test-race
```

bcrypt con coste 12 es lento bajo `-race`; las suites de auth y web tardan
notablemente más. Usa timeouts generosos en el cliente HTTP en modo carrera (30
segundos en la prueba de WebSocket) en lugar de reducir el coste de bcrypt en
producción.

## Ejecutor de integración

`make integration` compila el servidor y el cliente independiente `mailvoid-it`,
lanza MailVoid en puertos libres temporales y ejecuta la suite validada con JTD en
`testdata/integration`. Ejercita credenciales gestionadas, captura SMTP, REST,
MCP, WebDAV, descargas `.eml` y búsqueda Bleve. `make integration-tui` ofrece una
vista interactiva de selección y progreso de Charmbracelet.

Consulta [Ejecutor de integración](/development/integration-runner/) para el
esquema JSON, los tipos de paso, las aserciones y cómo añadir pruebas.

## Comprobación manual de extremo a extremo

```bash
make build
MAILVOID_ADMIN_PASSWORD=secret123 ./bin/mailvoid &

python3 - <<'PY'
import smtplib
from email.mime.text import MIMEText
m = MIMEText("hello")
m["Subject"] = "smoke test"
m["From"] = "a@b.test"
m["To"] = "x@y.test"
with smtplib.SMTP("127.0.0.1", 1025, timeout=5) as s:
    s.sendmail("a@b.test", ["x@y.test"], m.as_string())
PY

curl -s -c /tmp/cookies -d 'username=admin&password=secret123' \
  http://localhost:8025/login
curl -s -b /tmp/cookies http://localhost:8025/api/messages | jq
```

## Qué no está cubierto

- El renderizado del navegador y el comportamiento de JavaScript (HTMX, Cash,
  tema, cuenta atrás) se ejercitan manualmente o inspeccionando el HTML servido.
- Las pruebas de carga y de resistencia no forman parte de la suite.
- El proxy inverso externo, TLS y la integración con systemd son responsabilidades
  de despliegue.
