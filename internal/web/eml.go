package web

import (
	"mime"
	"strings"

	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gofiber/fiber/v2"
)

// handleMessageEML downloads the raw message from the session-protected console.
func (s *Server) handleMessageEML(c *fiber.Ctx) error {
	message, err := s.store.GetMessage(c.UserContext(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "message not found")
	}
	return sendEML(c, message)
}

// apiV1MessageEML downloads the raw message through the versioned API.
func (s *Server) apiV1MessageEML(c *fiber.Ctx) error {
	message, err := s.store.GetMessage(c.UserContext(), c.Params("id"))
	if err != nil {
		return apiError(c, fiber.StatusNotFound, "message not found")
	}
	return sendEML(c, message)
}

func sendEML(c *fiber.Ctx, message *models.Message) error {
	filename := emlFilename(message)
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	c.Set(fiber.HeaderContentType, "message/rfc822")
	if disposition != "" {
		c.Set(fiber.HeaderContentDisposition, disposition)
	}
	c.Set("X-Content-Type-Options", "nosniff")
	return c.SendString(message.Raw)
}

func emlFilename(message *models.Message) string {
	subject := strings.TrimSpace(message.Subject)
	var builder strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(subject) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == '.':
			if !lastDash {
				builder.WriteRune(r)
				lastDash = true
			}
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
		if builder.Len() >= 64 {
			break
		}
	}
	base := strings.Trim(builder.String(), "-._")
	if base == "" {
		base = "message"
	}
	id := strings.TrimPrefix(message.ID, "msg_")
	if id == "" {
		id = "unknown"
	}
	return base + "-" + id + ".eml"
}
