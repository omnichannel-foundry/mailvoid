package web

import (
	_ "embed"
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"gopkg.in/yaml.v3"
)

//go:embed openapi.yaml
var openAPISpec []byte

func (s *Server) handleOpenAPIYAML(c *fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, "application/yaml; charset=utf-8")
	c.Set(fiber.HeaderCacheControl, "public, max-age=300")
	return c.Send(openAPISpec)
}

func (s *Server) handleOpenAPIJSON(c *fiber.Ctx) error {
	var document any
	if err := yaml.Unmarshal(openAPISpec, &document); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "invalid embedded OpenAPI document")
	}
	body, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to encode OpenAPI document")
	}
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSONCharsetUTF8)
	c.Set(fiber.HeaderCacheControl, "public, max-age=300")
	return c.Send(body)
}
