package web

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/gnrfan/mailvoid/internal/mailbox"
	"github.com/gnrfan/mailvoid/internal/models"
	searchidx "github.com/gnrfan/mailvoid/internal/search"
	"github.com/gofiber/fiber/v2"
)

func parseSearchQuery(c *fiber.Ctx) (searchidx.Query, models.MessageFilter, error) {
	limit, offset, err := parseV1Pagination(c)
	if err != nil {
		return searchidx.Query{}, models.MessageFilter{}, err
	}
	since, err := parseV1Since(c, "since")
	if err != nil {
		return searchidx.Query{}, models.MessageFilter{}, err
	}
	until, err := parseV1Since(c, "until")
	if err != nil {
		return searchidx.Query{}, models.MessageFilter{}, err
	}
	action := strings.TrimSpace(c.Query("action"))
	if !validAction(action) {
		return searchidx.Query{}, models.MessageFilter{}, errors.New("action must be keep, ttl, or drop")
	}
	port := 0
	if raw := strings.TrimSpace(c.Query("port")); raw != "" {
		port, err = parsePort(raw)
		if err != nil {
			return searchidx.Query{}, models.MessageFilter{}, err
		}
	}
	sortMode := strings.TrimSpace(c.Query("sort"))
	switch sortMode {
	case "", "relevance":
		sortMode = "relevance"
	case "received_at":
	default:
		return searchidx.Query{}, models.MessageFilter{}, errors.New("sort must be relevance or received_at")
	}

	query := searchidx.Query{
		Text:      strings.TrimSpace(c.Query("q")),
		Mailbox:   strings.ToLower(strings.TrimSpace(c.Query("mailbox"))),
		Domain:    strings.ToLower(strings.TrimSpace(c.Query("domain"))),
		Recipient: strings.ToLower(strings.TrimSpace(c.Query("recipient"))),
		Sender:    strings.ToLower(strings.TrimSpace(c.Query("sender"))),
		Action:    action,
		Port:      port,
		Since:     since,
		Until:     until,
		Limit:     limit,
		Offset:    offset,
		Sort:      sortMode,
	}
	fallback := models.MessageFilter{
		Query:   query.Text,
		Mailbox: query.Mailbox,
		Domain:  query.Domain,
		Action:  query.Action,
		Port:    query.Port,
		Since:   query.Since,
		Until:   query.Until,
		Limit:   limit,
		Offset:  offset,
	}
	if query.Recipient != "" {
		mailboxName, domain, err := mailbox.ParseAddress(query.Recipient)
		if err != nil {
			return searchidx.Query{}, models.MessageFilter{}, err
		}
		fallback.Mailbox = mailboxName
		fallback.Domain = domain
	}
	if query.Sender != "" {
		fallback.Query = strings.TrimSpace(fallback.Query + " " + query.Sender)
	}
	return query, fallback, nil
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("port must be an integer between 1 and 65535")
	}
	return port, nil
}

func (s *Server) performSearch(ctx context.Context, query searchidx.Query, fallback models.MessageFilter) ([]*models.Message, int, map[string]map[string][]string, string, error) {
	if s.search != nil {
		results, err := s.search.Search(ctx, query)
		if err != nil {
			return nil, 0, nil, "", err
		}
		messages := make([]*models.Message, 0, len(results.Hits))
		highlights := make(map[string]map[string][]string)
		for _, hit := range results.Hits {
			message, err := s.store.GetMessage(ctx, hit.ID)
			if err != nil {
				continue
			}
			messages = append(messages, message)
			if len(hit.Highlights) > 0 {
				highlights[hit.ID] = hit.Highlights
			}
		}
		return messages, results.Total, highlights, "bleve", nil
	}

	messages, err := s.store.ListMessages(ctx, fallback)
	if err != nil {
		return nil, 0, nil, "", err
	}
	total, err := s.store.CountMessages(ctx, fallback)
	if err != nil {
		return nil, 0, nil, "", err
	}
	return messages, total, nil, "fallback", nil
}

func (s *Server) apiV1Search(c *fiber.Ctx) error {
	query, fallback, err := parseSearchQuery(c)
	if err != nil {
		return apiError(c, fiber.StatusBadRequest, err.Error())
	}
	messages, total, highlights, backend, err := s.performSearch(c.UserContext(), query, fallback)
	if err != nil {
		return err
	}
	out := make([]apiMessageSummary, 0, len(messages))
	for _, message := range messages {
		out = append(out, summarizeMessage(message))
	}
	response := fiber.Map{
		"messages": out,
		"count":    total,
		"limit":    query.Limit,
		"offset":   query.Offset,
		"backend":  backend,
	}
	if len(highlights) > 0 {
		response["highlights"] = highlights
	}
	return c.JSON(response)
}
