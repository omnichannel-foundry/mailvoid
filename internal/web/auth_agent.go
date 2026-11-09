package web

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// requireAgentAuth accepts either a configured bearer token or the normal
// session cookie. It is used by /api/v1 and /mcp.
func (s *Server) requireAgentAuth(c *fiber.Ctx) error {
	if header := strings.TrimSpace(c.Get(fiber.HeaderAuthorization)); header != "" {
		const prefix = "Bearer "
		if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
			c.Set(fiber.HeaderWWWAuthenticate, "Bearer")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid authorization header"})
		}
		token := strings.TrimSpace(header[len(prefix):])
		if s.cfg.APIToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.APIToken)) == 1 {
			c.Locals("user", "api-token")
			c.Locals("admin", true)
			return c.Next()
		}
		key, err := s.auth.AuthenticateAPIKey(c.UserContext(), token)
		if err != nil {
			c.Set(fiber.HeaderWWWAuthenticate, "Bearer")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid api token"})
		}
		c.Locals("user", "api-key:"+key.Name)
		c.Locals("admin", false)
		return c.Next()
	}

	sess, err := s.sessions.Get(c)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	user, _ := sess.Get("user").(string)
	if user == "" {
		c.Set(fiber.HeaderWWWAuthenticate, "Bearer")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}
	c.Locals("user", user)
	c.Locals("admin", true)
	return c.Next()
}

// requireAdmin requires a session administrator or the bootstrap API token.
// Managed API keys are deliberately not allowed to manage credentials.
func (s *Server) requireAdmin(c *fiber.Ctx) error {
	if admin, _ := c.Locals("admin").(bool); !admin {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "administrator privileges required"})
	}
	return c.Next()
}
