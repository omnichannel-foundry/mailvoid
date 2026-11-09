package web

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gnrfan/mailvoid/internal/mailbox"
	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gofiber/fiber/v2"
)

func (s *Server) handleAPIRoot(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"name":    "MailVoid API",
		"version": "1.0.0",
		"openapi": "/api/v1/openapi.json",
		"mcp":     "/mcp",
		"legacy":  "/api",
	})
}

func parseV1Since(c *fiber.Ctx, key string) (time.Time, error) {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, errors.New(key + " must be an RFC 3339 timestamp")
	}
	return parsed.UTC(), nil
}

func parseV1Pagination(c *fiber.Ctx) (limit, offset int, err error) {
	limit = 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 {
			return 0, 0, errors.New("limit must be a positive integer")
		}
		if limit > 200 {
			limit = 200
		}
	}
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			return 0, 0, errors.New("offset must be a non-negative integer")
		}
		return limit, offset, nil
	}
	page := 1
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 {
			return 0, 0, errors.New("page must be a positive integer")
		}
	}
	return limit, (page - 1) * limit, nil
}

func parseV1MessageFilter(c *fiber.Ctx) (models.MessageFilter, int, int, error) {
	limit, offset, err := parseV1Pagination(c)
	if err != nil {
		return models.MessageFilter{}, 0, 0, err
	}
	since, err := parseV1Since(c, "since")
	if err != nil {
		return models.MessageFilter{}, 0, 0, err
	}
	until, err := parseV1Since(c, "until")
	if err != nil {
		return models.MessageFilter{}, 0, 0, err
	}

	action := strings.TrimSpace(c.Query("action"))
	if !validAction(action) {
		return models.MessageFilter{}, 0, 0, errors.New("action must be keep, ttl, or drop")
	}
	port := 0
	if raw := strings.TrimSpace(c.Query("port")); raw != "" {
		port, err = strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return models.MessageFilter{}, 0, 0, errors.New("port must be an integer between 1 and 65535")
		}
	}

	filter := models.MessageFilter{
		Query:   strings.TrimSpace(c.Query("q")),
		Domain:  strings.ToLower(strings.TrimSpace(c.Query("domain"))),
		Mailbox: strings.ToLower(strings.TrimSpace(c.Query("mailbox"))),
		Action:  action,
		Port:    port,
		Since:   since,
		Until:   until,
		Limit:   limit,
		Offset:  offset,
	}
	if recipient := strings.TrimSpace(c.Query("recipient")); recipient != "" {
		mailboxName, domain, err := mailbox.ParseAddress(recipient)
		if err != nil {
			return models.MessageFilter{}, 0, 0, err
		}
		filter.Mailbox = mailboxName
		filter.Domain = domain
	}
	return filter, limit, offset, nil
}

func parseV1EventFilter(c *fiber.Ctx) (models.EventFilter, int, int, error) {
	limit, offset, err := parseV1Pagination(c)
	if err != nil {
		return models.EventFilter{}, 0, 0, err
	}
	since, err := parseV1Since(c, "since")
	if err != nil {
		return models.EventFilter{}, 0, 0, err
	}
	severity := strings.TrimSpace(c.Query("severity"))
	if severity != "" && !validSeverity(severity) {
		return models.EventFilter{}, 0, 0, errors.New("severity must be debug, info, warn, or error")
	}
	return models.EventFilter{
		Type:     strings.TrimSpace(c.Query("type")),
		Severity: severity,
		Query:    strings.TrimSpace(c.Query("q")),
		Since:    since,
		Limit:    limit,
		Offset:   offset,
	}, limit, offset, nil
}

func validSeverity(severity string) bool {
	for _, s := range severities {
		if s == severity {
			return true
		}
	}
	return false
}

func (s *Server) apiV1ListMessages(c *fiber.Ctx) error {
	filter, limit, offset, err := parseV1MessageFilter(c)
	if err != nil {
		return apiError(c, fiber.StatusBadRequest, err.Error())
	}
	messages, err := s.store.ListMessages(c.UserContext(), filter)
	if err != nil {
		return err
	}
	total, err := s.store.CountMessages(c.UserContext(), filter)
	if err != nil {
		return err
	}
	out := make([]apiMessageSummary, 0, len(messages))
	for _, message := range messages {
		out = append(out, summarizeMessage(message))
	}
	return c.JSON(fiber.Map{
		"messages": out,
		"count":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

func (s *Server) apiV1ListEvents(c *fiber.Ctx) error {
	filter, limit, offset, err := parseV1EventFilter(c)
	if err != nil {
		return apiError(c, fiber.StatusBadRequest, err.Error())
	}
	evs, err := s.store.ListEvents(c.UserContext(), filter)
	if err != nil {
		return err
	}
	total, err := s.store.CountEvents(c.UserContext(), filter)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"events": evs,
		"count":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) apiV1ListMailboxes(c *fiber.Ctx) error {
	limit, offset, err := parseV1Pagination(c)
	if err != nil {
		return apiError(c, fiber.StatusBadRequest, err.Error())
	}
	since, err := parseV1Since(c, "since")
	if err != nil {
		return apiError(c, fiber.StatusBadRequest, err.Error())
	}
	summaries, total, err := s.mailboxes.List(c.UserContext(), mailbox.ListOptions{
		Domain: strings.ToLower(strings.TrimSpace(c.Query("domain"))),
		Since:  since,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"mailboxes": summaries,
		"count":     total,
		"limit":     limit,
		"offset":    offset,
	})
}

func (s *Server) apiV1GetMailbox(c *fiber.Ctx) error {
	since, err := parseV1Since(c, "since")
	if err != nil {
		return apiError(c, fiber.StatusBadRequest, err.Error())
	}
	summary, err := s.mailboxes.Summary(c.UserContext(), c.Params("address"), since)
	if err != nil {
		if errors.Is(err, mailbox.ErrInvalidAddress) {
			return apiError(c, fiber.StatusBadRequest, err.Error())
		}
		return err
	}
	return c.JSON(summary)
}

func (s *Server) apiV1MailboxMessages(c *fiber.Ctx) error {
	filter, limit, offset, err := parseV1MessageFilter(c)
	if err != nil {
		return apiError(c, fiber.StatusBadRequest, err.Error())
	}
	mailboxName, domain, err := mailbox.ParseAddress(c.Params("address"))
	if err != nil {
		return apiError(c, fiber.StatusBadRequest, err.Error())
	}
	filter.Mailbox = mailboxName
	filter.Domain = domain

	messages, err := s.store.ListMessages(c.UserContext(), filter)
	if err != nil {
		return err
	}
	total, err := s.store.CountMessages(c.UserContext(), filter)
	if err != nil {
		return err
	}
	out := make([]apiMessageSummary, 0, len(messages))
	for _, message := range messages {
		out = append(out, summarizeMessage(message))
	}
	return c.JSON(fiber.Map{
		"messages": out,
		"count":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

func (s *Server) apiV1WaitMailbox(c *fiber.Ctx) error {
	since, err := parseV1Since(c, "since")
	if err != nil {
		return apiError(c, fiber.StatusBadRequest, err.Error())
	}
	timeout := 30 * time.Second
	if raw := strings.TrimSpace(c.Query("timeout")); raw != "" {
		timeout, err = time.ParseDuration(raw)
		if err != nil || timeout <= 0 {
			return apiError(c, fiber.StatusBadRequest, "timeout must be a positive Go duration")
		}
	}
	started := time.Now()
	message, err := s.mailboxes.Wait(c.Context(), c.Params("address"), since, timeout)
	waited := time.Since(started).Seconds()
	if err != nil {
		if errors.Is(err, mailbox.ErrTimeout) {
			return c.JSON(fiber.Map{"found": false, "waited_seconds": waited})
		}
		if errors.Is(err, mailbox.ErrInvalidAddress) {
			return apiError(c, fiber.StatusBadRequest, err.Error())
		}
		return err
	}
	return c.JSON(fiber.Map{
		"found":          true,
		"waited_seconds": waited,
		"message":        summarizeMessage(message),
	})
}
