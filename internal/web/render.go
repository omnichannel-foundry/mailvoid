package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

//go:embed templates
var templateFiles embed.FS

//go:embed static
var staticFiles embed.FS

// Renderer implements fiber.Views with embedded html/template pages and HTMX
// fragments.
type Renderer struct {
	pages map[string]*template.Template
	frags *template.Template
}

var pageLayouts = map[string]string{
	"dashboard":      "base.html",
	"messages":       "base.html",
	"message_detail": "base.html",
	"events":         "base.html",
	"policies":       "base.html",
	"settings":       "base.html",
	"login":          "auth.html",
	"error":          "auth.html",
}

// NewRenderer parses and caches every page and fragment.
func NewRenderer() (*Renderer, error) {
	funcs := templateFuncs()

	fragmentFiles, err := templateFiles.ReadDir("templates/fragments")
	if err != nil {
		return nil, err
	}
	fragmentPaths := make([]string, 0, len(fragmentFiles))
	for _, f := range fragmentFiles {
		if !f.IsDir() {
			fragmentPaths = append(fragmentPaths, "templates/fragments/"+f.Name())
		}
	}

	r := &Renderer{pages: make(map[string]*template.Template, len(pageLayouts))}
	for page, layout := range pageLayouts {
		files := []string{"templates/" + layout, "templates/partials.html"}
		files = append(files, fragmentPaths...)
		files = append(files, "templates/pages/"+page+".html")
		tmpl, err := template.New(layout).Funcs(funcs).ParseFS(templateFiles, files...)
		if err != nil {
			return nil, fmt.Errorf("parse page %s: %w", page, err)
		}
		r.pages[page] = tmpl
	}

	fragFiles := append([]string{"templates/partials.html"}, fragmentPaths...)
	frag, err := template.New("fragments").Funcs(funcs).ParseFS(templateFiles, fragFiles...)
	if err != nil {
		return nil, fmt.Errorf("parse fragments: %w", err)
	}
	r.frags = frag
	return r, nil
}

// Load implements fiber.Views.
func (r *Renderer) Load() error { return nil }

// Render implements fiber.Views.
func (r *Renderer) Render(w io.Writer, name string, bind interface{}, _ ...string) error {
	data := map[string]any{}
	if bind != nil {
		switch m := bind.(type) {
		case map[string]any:
			data = m
		case fiber.Map:
			for k, v := range m {
				data[k] = v
			}
		default:
			data["Data"] = bind
		}
	}

	if tmpl, ok := r.pages[name]; ok {
		layout := strings.TrimSuffix(pageLayouts[name], ".html")
		return tmpl.ExecuteTemplate(w, layout, data)
	}
	if r.frags != nil && r.frags.Lookup(name) != nil {
		return r.frags.ExecuteTemplate(w, name, data)
	}
	return fmt.Errorf("template %q not found", name)
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"formatTime": func(t time.Time) string {
			if t.IsZero() {
				return "—"
			}
			return t.UTC().Format("2006-01-02 15:04:05")
		},
		"formatTimeShort": func(t time.Time) string {
			if t.IsZero() {
				return "—"
			}
			return t.UTC().Format("15:04:05")
		},
		"relTime": func(t time.Time) string {
			if t.IsZero() {
				return "—"
			}
			return relativeTime(t)
		},
		"expiresIn": func(t *time.Time) string {
			if t == nil {
				return "permanent"
			}
			return relativeTime(*t)
		},
		"severityClass": func(s string) string {
			switch s {
			case "error":
				return "badge-error"
			case "warn":
				return "badge-warning"
			case "debug":
				return "badge-ghost"
			default:
				return "badge-info"
			}
		},
		"actionClass": func(a string) string {
			switch a {
			case "drop":
				return "badge-error"
			case "ttl":
				return "badge-warning"
			default:
				return "badge-success"
			}
		},
		"jsonPretty": func(v any) string {
			b, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return "{}"
			}
			return string(b)
		},
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "…"
		},
		"join": func(sep string, values []string) string { return strings.Join(values, sep) },
		"add":  func(a, b int) int { return a + b },
		"sub":  func(a, b int) int { return a - b },
		"mul":  func(a, b int) int { return a * b },
		"shortID": func(id string) string {
			if len(id) <= 10 {
				return id
			}
			return id[:10]
		},
		"rfc3339": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return t.UTC().Format(time.RFC3339)
		},
		"percent": func(part, total int) int {
			if total == 0 {
				return 0
			}
			return part * 100 / total
		},
		"defaultStr": func(fallback, value string) string {
			if strings.TrimSpace(value) == "" {
				return fallback
			}
			return value
		},
		"initial": func(value string) string {
			value = strings.TrimSpace(value)
			if value == "" {
				return "?"
			}
			for _, r := range strings.ToUpper(value) {
				return string(r)
			}
			return "?"
		},
	}
}

func relativeTime(t time.Time) string {
	now := time.Now().UTC()
	d := t.Sub(now)
	future := d > 0
	if !future {
		d = -d
	}
	var value string
	switch {
	case d < time.Minute:
		value = fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		value = fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		value = fmt.Sprintf("%dh", int(d.Hours()))
	default:
		value = fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	if future {
		return "in " + value
	}
	return value + " ago"
}
