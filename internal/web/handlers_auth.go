package web

import (
	"net/url"
	"strings"

	"github.com/gnrfan/mailvoid/internal/auth"
	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gofiber/fiber/v2"
)

func (s *Server) handleLoginGet(c *fiber.Ctx) error {
	sess, err := s.sessions.Get(c)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	if user, _ := sess.Get("user").(string); user != "" {
		return c.Redirect("/", fiber.StatusFound)
	}
	return s.render(c, "login", fiber.Map{
		"Title":    "Sign in",
		"Next":     safeNext(c.Query("next")),
		"Username": "",
	})
}

func (s *Server) handleLoginPost(c *fiber.Ctx) error {
	username := strings.TrimSpace(c.FormValue("username"))
	password := c.FormValue("password")
	next := safeNext(c.FormValue("next"))

	if username == "" || password == "" {
		return s.loginFailure(c, username, next, "Username and password are required.")
	}

	user, err := s.auth.Authenticate(c.UserContext(), username, password)
	if err != nil {
		if err == auth.ErrInvalidCredentials {
			return s.loginFailure(c, username, next, "Invalid username or password.")
		}
		return err
	}

	sess, err := s.sessions.Get(c)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	if err := sess.Regenerate(); err != nil {
		return fiber.ErrInternalServerError
	}
	sess.Set("user", user.Username)
	if err := sess.Save(); err != nil {
		return fiber.ErrInternalServerError
	}

	s.bus.Emit(events.New(events.TypeAuthLoginSuccess, events.SeverityInfo, map[string]any{
		"username":   user.Username,
		"remote_ip":  c.IP(),
		"user_agent": c.Get(fiber.HeaderUserAgent),
	}))
	return c.Redirect(next, fiber.StatusFound)
}

func (s *Server) loginFailure(c *fiber.Ctx, username, next, message string) error {
	s.bus.Emit(events.New(events.TypeAuthLoginFailed, events.SeverityWarn, map[string]any{
		"username":   username,
		"remote_ip":  c.IP(),
		"user_agent": c.Get(fiber.HeaderUserAgent),
	}))
	c.Status(fiber.StatusUnauthorized)
	return s.render(c, "login", fiber.Map{
		"Title":    "Sign in",
		"Next":     next,
		"Username": username,
		"Error":    message,
	})
}

func (s *Server) handleLogout(c *fiber.Ctx) error {
	user, _ := c.Locals("user").(string)
	sess, err := s.sessions.Get(c)
	if err == nil {
		_ = sess.Destroy()
	}
	s.bus.Emit(events.New(events.TypeAuthLogout, events.SeverityInfo, map[string]any{
		"username":  user,
		"remote_ip": c.IP(),
	}))
	return c.Redirect("/login", fiber.StatusFound)
}

func (s *Server) handlePasswordChange(c *fiber.Ctx) error {
	user, _ := c.Locals("user").(string)
	current := c.FormValue("current")
	password := c.FormValue("password")
	confirm := c.FormValue("confirm")

	fail := func(msg string) error {
		return c.Redirect("/settings?error="+url.QueryEscape(msg), fiber.StatusFound)
	}
	if len(password) < 8 {
		return fail("New password must be at least 8 characters.")
	}
	if password != confirm {
		return fail("New password and confirmation do not match.")
	}
	if _, err := s.auth.Authenticate(c.UserContext(), user, current); err != nil {
		if err == auth.ErrInvalidCredentials {
			return fail("Current password is incorrect.")
		}
		return err
	}
	if err := s.auth.SetPassword(c.UserContext(), user, password); err != nil {
		return err
	}
	s.bus.Emit(events.New(events.TypeAuthPasswordChanged, events.SeverityInfo, map[string]any{
		"username":  user,
		"remote_ip": c.IP(),
	}))
	return c.Redirect("/settings?flash="+url.QueryEscape("Password updated."), fiber.StatusFound)
}
