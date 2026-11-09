package web

import (
	"errors"
	"strings"
	"time"

	"github.com/gnrfan/mailvoid/internal/auth"
	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gofiber/fiber/v2"
)

type apiKeyView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedBy string     `json:"created_by,omitempty"`
}

func apiKeyToView(key *models.APIKey) apiKeyView {
	return apiKeyView{
		ID:        key.ID,
		Name:      key.Name,
		Prefix:    key.Prefix,
		CreatedAt: key.CreatedAt,
		ExpiresAt: key.ExpiresAt,
		CreatedBy: key.CreatedBy,
	}
}

type createAPIKeyInput struct {
	Name      string `json:"name"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type createdAPIKeyView struct {
	apiKeyView
	Token string `json:"token"`
}

func (s *Server) apiV1ListAPIKeys(c *fiber.Ctx) error {
	keys, err := s.store.ListAPIKeys(c.UserContext())
	if err != nil {
		return err
	}
	out := make([]apiKeyView, 0, len(keys))
	for _, key := range keys {
		out = append(out, apiKeyToView(key))
	}
	return c.JSON(fiber.Map{"api_keys": out, "count": len(out)})
}

func (s *Server) apiV1CreateAPIKey(c *fiber.Ctx) error {
	var input createAPIKeyInput
	if err := c.BodyParser(&input); err != nil {
		return apiError(c, fiber.StatusBadRequest, "invalid JSON body")
	}
	var expiresAt *time.Time
	if strings.TrimSpace(input.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, input.ExpiresAt)
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "expires_at must be an RFC 3339 timestamp")
		}
		expiresAt = &parsed
	}
	token, key, err := s.auth.CreateAPIKey(c.UserContext(), input.Name, expiresAt, localUser(c))
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredential) {
			return apiError(c, fiber.StatusBadRequest, "name is required and expires_at must be in the future")
		}
		return err
	}
	s.bus.Emit(events.New(events.TypeAPIKeyCreated, events.SeverityInfo, map[string]any{
		"key_id": key.ID, "name": key.Name, "prefix": key.Prefix,
		"expires_at": key.ExpiresAt, "username": localUser(c),
	}))
	return c.Status(fiber.StatusCreated).JSON(createdAPIKeyView{apiKeyView: apiKeyToView(key), Token: token})
}

func (s *Server) apiV1DeleteAPIKey(c *fiber.Ctx) error {
	key, err := s.store.GetAPIKey(c.UserContext(), c.Params("id"))
	if err != nil {
		return apiError(c, fiber.StatusNotFound, "api key not found")
	}
	if err := s.store.DeleteAPIKey(c.UserContext(), key.ID); err != nil {
		return err
	}
	s.bus.Emit(events.New(events.TypeAPIKeyDeleted, events.SeverityInfo, map[string]any{
		"key_id": key.ID, "name": key.Name, "prefix": key.Prefix, "username": localUser(c),
	}))
	return c.JSON(fiber.Map{"deleted": true, "id": key.ID})
}

type webDAVUserView struct {
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

type createWebDAVUserInput struct {
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
}

type createdWebDAVUserView struct {
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	Generated bool      `json:"generated"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) apiV1ListWebDAVUsers(c *fiber.Ctx) error {
	users, err := s.store.ListWebDAVUsers(c.UserContext())
	if err != nil {
		return err
	}
	out := make([]webDAVUserView, 0, len(users))
	for _, user := range users {
		out = append(out, webDAVUserView{Username: user.Username, CreatedAt: user.CreatedAt})
	}
	return c.JSON(fiber.Map{"webdav_users": out, "count": len(out)})
}

func (s *Server) apiV1CreateWebDAVUser(c *fiber.Ctx) error {
	var input createWebDAVUserInput
	if err := c.BodyParser(&input); err != nil {
		return apiError(c, fiber.StatusBadRequest, "invalid JSON body")
	}
	username := strings.ToLower(strings.TrimSpace(input.Username))
	if s.cfg.WebDAVPassword != "" && username == strings.ToLower(s.cfg.WebDAVUser) {
		return apiError(c, fiber.StatusConflict, "username is reserved by the bootstrap WebDAV credential")
	}
	generated, user, err := s.auth.CreateWebDAVUser(c.UserContext(), username, input.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredential) {
			return apiError(c, fiber.StatusBadRequest, "username is required and password must be at least 12 characters")
		}
		return err
	}
	s.webdavEnabled.Store(true)
	s.bus.Emit(events.New(events.TypeWebDAVUserCreated, events.SeverityInfo, map[string]any{
		"username": user.Username, "generated": generated != "", "username_actor": localUser(c),
	}))
	return c.Status(fiber.StatusCreated).JSON(createdWebDAVUserView{
		Username: user.Username, Password: choosePassword(input.Password, generated),
		Generated: generated != "", CreatedAt: user.CreatedAt,
	})
}

func (s *Server) apiV1DeleteWebDAVUser(c *fiber.Ctx) error {
	username := strings.ToLower(strings.TrimSpace(c.Params("username")))
	if err := s.store.DeleteWebDAVUser(c.UserContext(), username); err != nil {
		return apiError(c, fiber.StatusNotFound, "webdav user not found")
	}
	s.auth.InvalidateWebDAVCache()
	if s.cfg.WebDAVPassword == "" {
		if users, err := s.store.ListWebDAVUsers(c.UserContext()); err == nil && len(users) == 0 {
			s.webdavEnabled.Store(false)
		}
	}
	s.bus.Emit(events.New(events.TypeWebDAVUserDeleted, events.SeverityInfo, map[string]any{
		"username": username, "username_actor": localUser(c),
	}))
	return c.JSON(fiber.Map{"deleted": true, "username": username})
}

func choosePassword(provided, generated string) string {
	if generated != "" {
		return generated
	}
	return provided
}

func localUser(c *fiber.Ctx) string {
	user, _ := c.Locals("user").(string)
	return user
}
