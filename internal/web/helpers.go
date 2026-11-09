package web

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/policy"
	"github.com/gofiber/fiber/v2"
)

type messageFilterView struct {
	Q       string
	Domain  string
	Mailbox string
	Action  string
	Port    int
}

type eventFilterView struct {
	Type     string
	Severity string
	Query    string
}

type pagination struct {
	Page    int
	Pages   int
	Limit   int
	Total   int
	PrevURL string
	NextURL string
	HasPrev bool
	HasNext bool
}

type policyFormView struct {
	ID         string
	Name       string
	Field      string
	Pattern    string
	Action     string
	TTLSeconds int64
	Enabled    bool
	IsEdit     bool
}

func paginationParams(c *fiber.Ctx) (page, limit int) {
	page, _ = strconv.Atoi(c.Query("page", "1"))
	limit, _ = strconv.Atoi(c.Query("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return page, limit
}

func buildPagination(c *fiber.Ctx, base string, page, limit, total int) pagination {
	pages := (total + limit - 1) / limit
	if pages < 1 {
		pages = 1
	}
	if page > pages {
		page = pages
	}
	p := pagination{Page: page, Pages: pages, Limit: limit, Total: total}

	values := url.Values{}
	for k, v := range c.Queries() {
		if k == "page" {
			continue
		}
		values.Set(k, v)
	}
	values.Set("limit", strconv.Itoa(limit))

	if page > 1 {
		values.Set("page", strconv.Itoa(page-1))
		p.PrevURL = base + "?" + values.Encode()
		p.HasPrev = true
	}
	if page < pages {
		values.Set("page", strconv.Itoa(page+1))
		p.NextURL = base + "?" + values.Encode()
		p.HasNext = true
	}
	return p
}

func parseMessageFilter(c *fiber.Ctx) (models.MessageFilter, messageFilterView, pagination) {
	page, limit := paginationParams(c)
	view := messageFilterView{
		Q:       strings.TrimSpace(c.Query("q")),
		Domain:  strings.ToLower(strings.TrimSpace(c.Query("domain"))),
		Mailbox: strings.ToLower(strings.TrimSpace(c.Query("mailbox"))),
		Action:  strings.TrimSpace(c.Query("action")),
	}
	if !validAction(view.Action) {
		view.Action = ""
	}
	if port, err := strconv.Atoi(c.Query("port")); err == nil {
		view.Port = port
	}
	filter := models.MessageFilter{
		Query:   view.Q,
		Domain:  view.Domain,
		Mailbox: view.Mailbox,
		Action:  view.Action,
		Port:    view.Port,
		Limit:   limit,
		Offset:  (page - 1) * limit,
	}
	return filter, view, pagination{Page: page, Limit: limit}
}

func parseEventFilter(c *fiber.Ctx) (models.EventFilter, eventFilterView, pagination) {
	page, limit := paginationParams(c)
	if limit > 200 {
		limit = 200
	}
	view := eventFilterView{
		Type:     strings.TrimSpace(c.Query("type")),
		Severity: strings.TrimSpace(c.Query("severity")),
		Query:    strings.TrimSpace(c.Query("q")),
	}
	filter := models.EventFilter{
		Type:     view.Type,
		Severity: view.Severity,
		Query:    view.Query,
		Limit:    limit,
		Offset:   (page - 1) * limit,
	}
	return filter, view, pagination{Page: page, Limit: limit}
}

func parsePolicyForm(c *fiber.Ctx) (*models.Policy, error) {
	ttl, err := strconv.ParseInt(strings.TrimSpace(c.FormValue("ttl_seconds")), 10, 64)
	if err != nil && strings.TrimSpace(c.FormValue("ttl_seconds")) != "" {
		return nil, errors.New("TTL must be a whole number of seconds")
	}
	enabled := false
	switch strings.ToLower(c.FormValue("enabled")) {
	case "1", "on", "true", "yes":
		enabled = true
	}
	p := &models.Policy{
		Name:       strings.TrimSpace(c.FormValue("name")),
		Field:      strings.TrimSpace(c.FormValue("field")),
		Pattern:    c.FormValue("pattern"),
		Action:     strings.TrimSpace(c.FormValue("action")),
		TTLSeconds: ttl,
		Enabled:    enabled,
	}
	if err := policy.Validate(p); err != nil {
		return nil, err
	}
	return p, nil
}

func policyFormFromModel(p *models.Policy) policyFormView {
	if p == nil {
		return policyFormView{Enabled: true, Action: models.ActionKeep, Field: models.FieldSender}
	}
	return policyFormView{
		ID:         p.ID,
		Name:       p.Name,
		Field:      p.Field,
		Pattern:    p.Pattern,
		Action:     p.Action,
		TTLSeconds: p.TTLSeconds,
		Enabled:    p.Enabled,
		IsEdit:     true,
	}
}

func validAction(action string) bool {
	if action == "" {
		return true
	}
	for _, a := range models.PolicyActions {
		if a == action {
			return true
		}
	}
	return false
}

func safeNext(next string) string {
	next = strings.TrimSpace(next)
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	if strings.ContainsAny(next, "\\\r\n") {
		return "/"
	}
	return next
}

func apiError(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(fiber.Map{"error": message})
}
