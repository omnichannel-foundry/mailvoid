---
title: Frontend and assets
weight: 30
---

# Frontend and assets

The frontend is server-rendered Go `html/template` with HTMX for partial
updates, DaisyUI/Tailwind for styling, and Cash for small DOM helpers. There is
no Node.js build step at development or runtime.

## Template structure

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

Pages define `{{define "content"}}`; the layout executes `{{template "content" .}}`.
Fragments define a named template that is executed directly by the renderer.

## Renderer

`internal/web/render.go` embeds the whole template tree. At startup it builds:

- one cached template set per page: layout + `partials.html` + all fragments +
  the page file;
- one fragment set containing `partials.html` + all fragments.

`Renderer.Render` looks up the page name first, selects `base` or `auth` as the
layout, and executes it. If the name is not a page, it executes a fragment. This
lets handlers call:

```go
s.render(c, "messages", data)          // full page
s.renderFragment(c, "message_list", v) // HTMX partial
```

### Common data

`s.render` injects:

| Key | Value |
|-----|-------|
| `Version` | build version |
| `Active` | sidebar active item |
| `User` | session username |
| `Flash` | `?flash=` message |
| `Error` | `?error=` message |
| `Title` | page title |

Pages add their own keys (`Messages`, `Filter`, `Pagination`, `Stats`, ...).

### Template functions

`templateFuncs` provides `formatTime`, `relTime`, `expiresIn`, `rfc3339`,
`severityClass`, `actionClass`, `jsonPretty`, `truncate`, `join`, `add`, `sub`,
`mul`, `shortID`, `percent`, and `defaultStr`.

## HTMX

The dependency is pinned at `htmx.org@2.0.4` and embedded at
`static/htmx.min.js`.

### Message filtering

The filter form uses:

```html
<form hx-get="/messages"
      hx-target="#message-list"
      hx-swap="outerHTML"
      hx-push-url="true"
      hx-trigger="submit, change, keyup changed delay:350ms from:input[name=q]">
```

The handler returns the `message_list` fragment when `HX-Request: true`,
otherwise the full page. The fragment contains the table and pagination.

### Policy editing and testing

- **Edit** issues `hx-get="/policies/:id/edit"` and replaces the form panel.
- **Regex tester** issues `hx-post="/policies/test"` and renders the
  `policy_test` fragment.

### Adding a fragment

1. Create `templates/fragments/name.html` with `{{define "name"}}...{{end}}`.
2. The renderer discovers it automatically at startup.
3. Return it with `s.renderFragment(c, "name", data)`.
4. If the fragment uses pagination or flash partials, they are already
   available.

## Cash

Cash (`cash-dom@8.1.5`) is used in `static/app.js` for:

- theme toggle and persistence in `localStorage`;
- clipboard copying from `data-copy-target` buttons;
- closing the mobile drawer after navigation.

The app also uses native DOM APIs for event-row construction and countdowns.

## Theme

- `data-theme` is `dark` or `light`; the initial value is `dark`.
- An inline script in `<head>` applies the stored theme before paint to avoid a
  flash.
- The toggle is `#theme-toggle`; the value is stored under `mailvoid-theme`.

DaisyUI full CSS provides the themes and Tailwind utilities. Custom styles live
in `static/app.css` under the `mv-` prefix.

## Embedded assets

`internal/web/static` is embedded with `go:embed` and served by
`handleStatic`:

| File | Version |
|------|---------|
| `daisyui.min.css` | `daisyui@4.12.24/dist/full.min.css` |
| `htmx.min.js` | `htmx.org@2.0.4` |
| `cash.min.js` | `cash-dom@8.1.5` |
| `app.css`, `app.js` | local |

To upgrade an asset:

1. Download the pinned minified file.
2. Replace the file in `internal/web/static`.
3. Update this table when the version changes.
4. Rebuild and check the console visually.

The full DaisyUI CSS is about 2.9 MiB. It is cached for one hour. If binary size
or first-load cost becomes a problem, replace it with a Tailwind build that
includes only used utilities and components.

## Security

- Email HTML is rendered in `<iframe sandbox srcdoc="...">` without scripts or
  same-origin access.
- All other user content is escaped by `html/template`.
- Event payloads are pretty-printed JSON and escaped.
- Clipboard buttons copy text content, not HTML.

## Live events client

`static/app.js`:

- connects to `ws://` or `wss://` `/ws/events`;
- prepends event rows, capped at 500 DOM nodes;
- reconnects with exponential backoff up to 15 seconds;
- pauses and buffers up to 200 events in the browser;
- updates `.mv-live-dot` state.

Countdowns read `data-expires` RFC 3339 values and update every second.

## Build and preview

There is no frontend build command. Changes to templates or static files require
a Go rebuild because they are embedded:

```bash
make build
./bin/mailvoid
```
