// Package web implements the Fiber HTTP server, SSR dashboard, JSON API, and
// live event transport.
package web

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gnrfan/mailvoid/internal/auth"
	"github.com/gnrfan/mailvoid/internal/capture"
	"github.com/gnrfan/mailvoid/internal/config"
	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/mailbox"
	"github.com/gnrfan/mailvoid/internal/policy"
	searchidx "github.com/gnrfan/mailvoid/internal/search"
	"github.com/gnrfan/mailvoid/internal/storage"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/gofiber/websocket/v2"
	"golang.org/x/net/webdav"
)

// Server owns the HTTP application.
type Server struct {
	cfg           config.Config
	store         storage.Store
	bus           *events.Bus
	engine        *policy.Engine
	auth          *auth.Service
	mailboxes     *mailbox.Service
	renderer      *Renderer
	sessions      *session.Store
	app           *fiber.App
	mcpHTTP       http.Handler
	webdavHTTP    http.Handler
	search        searchidx.Index
	capture       *capture.Controller
	version       string
	startedAt     time.Time
	done          chan struct{}
	doneOnce      sync.Once
	webdavEnabled atomic.Bool
}

// New constructs the web server and its routes.
func New(cfg config.Config, store storage.Store, bus *events.Bus, engine *policy.Engine, version string) (*Server, error) {
	renderer, err := NewRenderer()
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg:       cfg,
		store:     store,
		bus:       bus,
		engine:    engine,
		auth:      auth.New(store),
		mailboxes: mailbox.New(store),
		renderer:  renderer,
		capture:   capture.New(true),
		version:   version,
		startedAt: time.Now().UTC(),
		done:      make(chan struct{}),
	}
	s.mcpHTTP = s.newMCPHandler()
	_ = mime.AddExtensionType(".eml", "message/rfc822")
	if cfg.WebDAVPassword != "" {
		s.webdavEnabled.Store(true)
	} else if users, listErr := store.ListWebDAVUsers(context.Background()); listErr == nil && len(users) > 0 {
		s.webdavEnabled.Store(true)
	}
	s.webdavHTTP = &webdav.Handler{
		Prefix:     "/dav",
		FileSystem: &davFS{store: store, mailboxes: s.mailboxes},
		LockSystem: webdav.NewMemLS(),
	}
	s.sessions = session.New(session.Config{
		Expiration:     12 * time.Hour,
		KeyLookup:      "cookie:mailvoid_session",
		CookiePath:     "/",
		CookieHTTPOnly: true,
		CookieSameSite: "Lax",
		CookieSecure:   cfg.SessionCookieSecure,
	})

	s.app = fiber.New(fiber.Config{
		AppName:               "MailVoid",
		DisableStartupMessage: true,
		BodyLimit:             8 << 20,
		Views:                 renderer,
		ErrorHandler:          s.handleError,
		RequestMethods:        append(append([]string{}, fiber.DefaultMethods...), webDAVMethods...),
	})
	s.routes()
	return s, nil
}

// App exposes the underlying Fiber app for tests.
func (s *Server) App() *fiber.App { return s.app }

// SetSearchIndex enables full-text search for the v1 API and MCP tools. A nil
// index keeps the substring-search fallback.
func (s *Server) SetSearchIndex(index searchidx.Index) { s.search = index }

// SetCaptureController shares the runtime capture switch with the SMTP server.
func (s *Server) SetCaptureController(controller *capture.Controller) { s.capture = controller }

// Start listens on the configured HTTP address.
func (s *Server) Start() error {
	return s.app.Listen(s.cfg.HTTPAddr)
}

// Shutdown gracefully stops the HTTP server and live streams.
func (s *Server) Shutdown(ctx context.Context) error {
	s.doneOnce.Do(func() { close(s.done) })
	done := make(chan error, 1)
	go func() { done <- s.app.Shutdown() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

var webDAVMethods = []string{"PROPFIND", "PROPPATCH", "MKCOL", "LOCK", "UNLOCK", "COPY", "MOVE"}

func (s *Server) routes() {
	app := s.app

	app.Use(recover.New())
	app.Get("/healthz", s.handleHealth)
	app.Get("/static/*", s.handleStatic)

	app.Get("/api/v1", s.handleAPIRoot)
	app.Get("/api/v1/openapi.yaml", s.handleOpenAPIYAML)
	app.Get("/api/v1/openapi.json", s.handleOpenAPIJSON)

	app.Use("/api/v1", s.requireAgentAuth)
	app.Get("/api/v1/mailboxes", s.apiV1ListMailboxes)
	app.Get("/api/v1/mailboxes/:address", s.apiV1GetMailbox)
	app.Get("/api/v1/mailboxes/:address/messages", s.apiV1MailboxMessages)
	app.Get("/api/v1/mailboxes/:address/wait", s.apiV1WaitMailbox)
	app.Get("/api/v1/messages", s.apiV1ListMessages)
	app.Get("/api/v1/search", s.apiV1Search)
	app.Get("/api/v1/messages/:id/eml", s.apiV1MessageEML)
	app.Get("/api/v1/messages/:id", s.apiGetMessage)
	app.Delete("/api/v1/messages/:id", s.apiDeleteMessage)
	app.Get("/api/v1/events", s.apiV1ListEvents)
	app.Get("/api/v1/policies", s.apiListPolicies)
	app.Post("/api/v1/policies", s.apiCreatePolicy)
	app.Post("/api/v1/policies/reorder", s.apiReorderPolicies)
	app.Post("/api/v1/policies/test", s.apiTestPolicy)
	app.Get("/api/v1/policies/:id", s.apiGetPolicy)
	app.Put("/api/v1/policies/:id", s.apiUpdatePolicy)
	app.Delete("/api/v1/policies/:id", s.apiDeletePolicy)
	app.Get("/api/v1/stats", s.apiStats)

	admin := app.Group("/api/v1/admin", s.requireAdmin)
	admin.Get("/api-keys", s.apiV1ListAPIKeys)
	admin.Post("/api-keys", s.apiV1CreateAPIKey)
	admin.Delete("/api-keys/:id", s.apiV1DeleteAPIKey)
	admin.Get("/webdav-users", s.apiV1ListWebDAVUsers)
	admin.Post("/webdav-users", s.apiV1CreateWebDAVUser)
	admin.Delete("/webdav-users/:username", s.apiV1DeleteWebDAVUser)

	app.All("/mcp", s.requireAgentAuth, adaptor.HTTPHandler(s.mcpHTTP))

	davHandler := adaptor.HTTPHandler(s.webdavHTTP)
	for _, method := range []string{
		fiber.MethodOptions, fiber.MethodGet, fiber.MethodHead,
		"PROPFIND", "PROPPATCH", "MKCOL", "LOCK", "UNLOCK", "COPY", "MOVE",
		fiber.MethodPut, fiber.MethodDelete, fiber.MethodPost,
	} {
		app.Add(method, "/dav", s.requireWebDAVAuth, webDAVReadOnly, davHandler)
		app.Add(method, "/dav/*", s.requireWebDAVAuth, webDAVReadOnly, davHandler)
	}

	app.Get("/login", s.handleLoginGet)
	app.Post("/login", s.handleLoginPost)

	app.Use(s.requireAuth)

	app.Get("/", s.handleDashboard)
	app.Get("/messages", s.handleMessages)
	app.Get("/messages/:id", s.handleMessageDetail)
	app.Get("/messages/:id/eml", s.handleMessageEML)
	app.Post("/messages/:id/delete", s.handleMessageDelete)
	app.Get("/events", s.handleEvents)
	app.Get("/policies", s.handlePolicies)
	app.Post("/policies", s.handlePolicyCreate)
	app.Post("/policies/test", s.handlePolicyTest)
	app.Get("/policies/:id/edit", s.handlePolicyEditForm)
	app.Post("/policies/:id", s.handlePolicyUpdate)
	app.Post("/policies/:id/toggle", s.handlePolicyToggle)
	app.Post("/policies/:id/move", s.handlePolicyMove)
	app.Post("/policies/:id/delete", s.handlePolicyDelete)
	app.Get("/settings", s.handleSettings)
	app.Post("/settings/capture", s.handleCaptureToggle)
	app.Post("/settings/password", s.handlePasswordChange)
	app.Post("/logout", s.handleLogout)

	app.Get("/api/messages", s.apiListMessages)
	app.Get("/api/messages/:id", s.apiGetMessage)
	app.Delete("/api/messages/:id", s.apiDeleteMessage)
	app.Get("/api/events", s.apiListEvents)
	app.Get("/api/events/stream", s.apiEventStream)
	app.Get("/api/policies", s.apiListPolicies)
	app.Post("/api/policies", s.apiCreatePolicy)
	app.Post("/api/policies/reorder", s.apiReorderPolicies)
	app.Post("/api/policies/test", s.apiTestPolicy)
	app.Get("/api/policies/:id", s.apiGetPolicy)
	app.Put("/api/policies/:id", s.apiUpdatePolicy)
	app.Delete("/api/policies/:id", s.apiDeletePolicy)
	app.Get("/api/stats", s.apiStats)

	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.NewError(fiber.StatusUpgradeRequired, "websocket upgrade required")
	})
	app.Get("/ws/events", websocket.New(s.handleWebSocket))
}

func (s *Server) handleHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "ok",
		"version": s.version,
		"uptime":  time.Since(s.startedAt).Round(time.Second).String(),
	})
}

func (s *Server) handleStatic(c *fiber.Ctx) error {
	rel := c.Params("*")
	rel = strings.TrimPrefix(path.Clean("/"+rel), "/")
	if rel == "" || strings.HasPrefix(rel, "..") {
		return fiber.ErrNotFound
	}
	data, err := fs.ReadFile(staticFiles, "static/"+rel)
	if err != nil {
		return fiber.ErrNotFound
	}
	contentType := mime.TypeByExtension(path.Ext(rel))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Set(fiber.HeaderContentType, contentType)
	c.Set(fiber.HeaderCacheControl, "public, max-age=3600")
	c.Set("X-Content-Type-Options", "nosniff")
	return c.Send(data)
}

func (s *Server) handleError(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "internal server error"
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		code = fiberErr.Code
		message = fiberErr.Message
	}
	if strings.HasPrefix(c.Path(), "/api/") || strings.HasPrefix(c.Path(), "/ws") {
		return c.Status(code).JSON(fiber.Map{"error": message})
	}
	if c.Response().StatusCode() != fiber.StatusOK {
		return c.SendStatus(code)
	}
	c.Status(code)
	if renderErr := s.render(c, "error", fiber.Map{
		"Title":   fmt.Sprintf("%d", code),
		"Status":  code,
		"Message": message,
	}); renderErr != nil {
		return c.SendString(fmt.Sprintf("%d %s", code, message))
	}
	return nil
}

func (s *Server) render(c *fiber.Ctx, page string, data fiber.Map) error {
	if data == nil {
		data = fiber.Map{}
	}
	if _, ok := data["Version"]; !ok {
		data["Version"] = s.version
	}
	if _, ok := data["Active"]; !ok {
		data["Active"] = activeFor(page)
	}
	if _, ok := data["User"]; !ok {
		data["User"] = c.Locals("user")
	}
	if _, ok := data["Flash"]; !ok {
		if flash := c.Query("flash"); flash != "" {
			data["Flash"] = flash
		}
	}
	if _, ok := data["Error"]; !ok {
		if msg := c.Query("error"); msg != "" {
			data["Error"] = msg
		}
	}
	if _, ok := data["Title"]; !ok {
		data["Title"] = titleFor(page)
	}
	if _, ok := data["CaptureEnabled"]; !ok {
		data["CaptureEnabled"] = s.capture.Enabled()
	}
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return c.Render(page, data)
}

func (s *Server) renderFragment(c *fiber.Ctx, name string, data fiber.Map) error {
	if data == nil {
		data = fiber.Map{}
	}
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return c.Render(name, data)
}

func (s *Server) requireAuth(c *fiber.Ctx) error {
	sess, err := s.sessions.Get(c)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	user, _ := sess.Get("user").(string)
	if user == "" {
		if strings.HasPrefix(c.Path(), "/api/") || strings.HasPrefix(c.Path(), "/ws") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
		}
		next := c.OriginalURL()
		if next == "" {
			next = "/"
		}
		return c.Redirect("/login?next="+url.QueryEscape(next), fiber.StatusFound)
	}
	c.Locals("user", user)
	return c.Next()
}

func activeFor(page string) string {
	switch page {
	case "message_detail":
		return "messages"
	case "dashboard", "messages", "events", "policies", "settings":
		return page
	default:
		return ""
	}
}

func titleFor(page string) string {
	switch page {
	case "dashboard":
		return "Dashboard"
	case "messages":
		return "Messages"
	case "message_detail":
		return "Message"
	case "events":
		return "Event stream"
	case "policies":
		return "Policies"
	case "settings":
		return "Settings"
	case "login":
		return "Sign in"
	case "error":
		return "Error"
	default:
		return "MailVoid"
	}
}
