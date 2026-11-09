---
title: Frontend y recursos estáticos
weight: 30
---

# Frontend y recursos estáticos

El frontend se renderiza en el servidor con `html/template` de Go, usa HTMX para
las actualizaciones parciales, DaisyUI/Tailwind para los estilos y Cash para
pequeños helpers del DOM. Las utilidades de Tailwind y los componentes de
DaisyUI se compilan de antemano en `internal/web/static/app.css`; Node.js solo
hace falta para regenerar ese archivo.

## Estructura de plantillas

```text
internal/web/templates/
├── base.html                 full console layout (sidebar, topbar, footer)
├── auth.html                 minimal layout for login and error pages
├── partials.html             topbar, sidebar, flash, pagination
├── pages/
│   ├── dashboard.html
│   ├── messages.html
│   ├── message_detail.html
│   ├── events.html
│   ├── policies.html
│   ├── settings.html
│   ├── login.html
│   └── error.html
└── fragments/
    ├── message_list.html
    ├── event_rows.html
    ├── policy_form.html
    └── policy_test.html
```

Las páginas definen `{{define "content"}}`; el layout ejecuta
`{{template "content" .}}`. Los fragmentos definen una plantilla con nombre que
el renderizador ejecuta directamente.

## Renderizador

`internal/web/render.go` incrusta todo el árbol de plantillas. Al arrancar
construye:

- un conjunto de plantillas en caché por página: layout + `partials.html` + todos
  los fragmentos + el archivo de la página;
- un conjunto de fragmentos que contiene `partials.html` + todos los fragmentos.

`Renderer.Render` busca primero el nombre de la página, selecciona `base` o
`auth` como layout y lo ejecuta. Si el nombre no es una página, ejecuta un
fragmento. Esto permite a los handlers llamar a:

```go
s.render(c, "messages", data)          // full page
s.renderFragment(c, "message_list", v) // HTMX partial
```

### Datos comunes

`s.render` inyecta:

| Clave | Valor |
|-------|-------|
| `Version` | versión de compilación |
| `Active` | elemento activo de la barra lateral |
| `User` | nombre de usuario de la sesión |
| `Flash` | mensaje de `?flash=` |
| `Error` | mensaje de `?error=` |
| `Title` | título de la página |

Las páginas añaden sus propias claves (`Messages`, `Filter`, `Pagination`,
`Stats`, ...).

### Funciones de plantilla

`templateFuncs` proporciona `formatTime`, `relTime`, `expiresIn`, `rfc3339`,
`severityClass`, `actionClass`, `jsonPretty`, `truncate`, `join`, `add`, `sub`,
`mul`, `shortID`, `percent` y `defaultStr`.

## HTMX

La dependencia está fijada en `htmx.org@2.0.4` e incrustada en
`static/htmx.min.js`.

### Filtrado de mensajes

El formulario de filtro usa:

```html
<form hx-get="/messages"
      hx-target="#message-list"
      hx-swap="outerHTML"
      hx-push-url="true"
      hx-trigger="submit, change, keyup changed delay:350ms from:input[name=q]">
```

El handler devuelve el fragmento `message_list` cuando `HX-Request: true`; en caso
contrario, la página completa. El fragmento contiene la tabla y la paginación.

### Edición y prueba de políticas

- **Editar** emite `hx-get="/policies/:id/edit"` y reemplaza el panel del
  formulario.
- **Probador de regex** emite `hx-post="/policies/test"` y renderiza el fragmento
  `policy_test`.

### Añadir un fragmento

1. Crea `templates/fragments/name.html` con `{{define "name"}}...{{end}}`.
2. El renderizador lo descubre automáticamente al arrancar.
3. Devuélvelo con `s.renderFragment(c, "name", data)`.
4. Si el fragmento usa parciales de paginación o flash, ya están disponibles.

## Cash

Cash (`cash-dom@8.1.5`) se usa en `static/app.js` para:

- alternar el tema y persistirlo en `localStorage`;
- copiar al portapapeles desde los botones `data-copy-target`;
- cerrar el cajón móvil después de la navegación.

La aplicación también usa las API nativas del DOM para construir las filas de
eventos y las cuentas atrás.

## Tema

- `data-theme` es `dark` o `light`; el valor inicial es `dark`.
- Un script en línea en `<head>` aplica el tema almacenado antes del primer
  pintado para evitar un destello.
- El conmutador es `#theme-toggle`; el valor se guarda bajo `mailvoid-theme`.

`internal/web/static-src/app.css` es la hoja de estilos fuente. Importa las capas
base, components y utilities de Tailwind, y luego define los estilos propios con
prefijo `mv-` y las sobreescrituras de tema. `tailwind.config.js` escanea las
plantillas y `app.js`, e incluye en la safelist las clases de badge que generan
los helpers de Go. `npm run build:css` escribe el resultado minificado en
`internal/web/static/app.css`. Ejecuta `make assets` después de cambiar
plantillas o estilos.

## Recursos incrustados

`internal/web/static` se incrusta con `go:embed` y lo sirve `handleStatic`:

| Archivo | Origen |
|---------|--------|
| `app.css` | Tailwind + DaisyUI + estilos propios generados |
| `htmx.min.js` | `htmx.org@2.0.4` |
| `cash.min.js` | `cash-dom@8.1.5` |
| `app.js` | local |

Para actualizar Tailwind o DaisyUI:

1. Actualiza las versiones en `package.json`.
2. Ejecuta `npm install` para refrescar `package-lock.json` y `node_modules`.
3. Ejecuta `make assets`.
4. Recompila y revisa visualmente la consola.

La hoja generada contiene solo los temas `dark` y `light`, los componentes de
DaisyUI en uso y las utilidades de Tailwind encontradas en las plantillas. Sigue
siendo mucho más pequeña que el bundle completo de DaisyUI.

## Seguridad

- El HTML del correo se renderiza en `<iframe sandbox srcdoc="...">` sin scripts
  ni acceso al mismo origen.
- Todo el resto del contenido del usuario lo escapa `html/template`.
- Los payloads de los eventos son JSON formateado y escapado.
- Los botones de portapapeles copian el contenido de texto, no HTML.

## Cliente de eventos en vivo

`static/app.js`:

- se conecta a `ws://` o `wss://` en `/ws/events`;
- antepone filas de eventos, con un límite de 500 nodos del DOM;
- se reconecta con retroceso exponencial de hasta 15 segundos;
- pausa y almacena en búfer hasta 200 eventos en el navegador;
- actualiza el estado de `.mv-live-dot`.

Las cuentas atrás leen valores RFC 3339 de `data-expires` y se actualizan cada
segundo.

## Compilación y previsualización

No hay un comando de compilación del frontend. Los cambios en plantillas o
archivos estáticos requieren recompilar Go porque están incrustados:

```bash
make build
./bin/mailvoid
```
