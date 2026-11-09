package web

import (
	"context"
	"net/url"
	"time"

	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/policy"
	"github.com/gofiber/fiber/v2"
)

type dashboardStats struct {
	TotalMessages int
	Last24h       int
	Policies      int
	Events        int
}

var eventTypes = []string{
	events.TypeSystemStartup,
	events.TypeSystemShutdown,
	events.TypeSMTPConnected,
	events.TypeSMTPClosed,
	events.TypeSMTPMailFrom,
	events.TypeSMTPRcptTo,
	events.TypeSMTPData,
	events.TypeSMTPError,
	events.TypeMessageReceived,
	events.TypeMessageDropped,
	events.TypeMessageDeleted,
	events.TypeMessageError,
	events.TypePolicyMatched,
	events.TypePolicyCreated,
	events.TypePolicyUpdated,
	events.TypePolicyDeleted,
	events.TypePolicyReordered,
	events.TypeAuthLoginSuccess,
	events.TypeAuthLoginFailed,
	events.TypeAuthLogout,
	events.TypeAuthPasswordChanged,
	events.TypeAPIKeyCreated,
	events.TypeAPIKeyDeleted,
	events.TypeWebDAVUserCreated,
	events.TypeWebDAVUserDeleted,
	events.TypeSearchError,
}

var severities = []string{
	events.SeverityDebug,
	events.SeverityInfo,
	events.SeverityWarn,
	events.SeverityError,
}

func (s *Server) gatherStats(ctx context.Context) (dashboardStats, error) {
	total, err := s.store.CountMessages(ctx, models.MessageFilter{})
	if err != nil {
		return dashboardStats{}, err
	}
	last24, err := s.store.CountMessages(ctx, models.MessageFilter{Since: time.Now().UTC().Add(-24 * time.Hour)})
	if err != nil {
		return dashboardStats{}, err
	}
	policies, err := s.store.ListPolicies(ctx)
	if err != nil {
		return dashboardStats{}, err
	}
	eventCount, err := s.store.CountEvents(ctx, models.EventFilter{})
	if err != nil {
		return dashboardStats{}, err
	}
	return dashboardStats{
		TotalMessages: total,
		Last24h:       last24,
		Policies:      len(policies),
		Events:        eventCount,
	}, nil
}

func (s *Server) handleDashboard(c *fiber.Ctx) error {
	ctx := c.UserContext()
	stats, err := s.gatherStats(ctx)
	if err != nil {
		return err
	}
	recentMessages, err := s.store.ListMessages(ctx, models.MessageFilter{Limit: 8})
	if err != nil {
		return err
	}
	recentEvents, err := s.store.ListEvents(ctx, models.EventFilter{Limit: 8})
	if err != nil {
		return err
	}

	return s.render(c, "dashboard", fiber.Map{
		"Title":          "Dashboard",
		"Stats":          stats,
		"RecentMessages": recentMessages,
		"RecentEvents":   recentEvents,
	})
}

func (s *Server) handleMessages(c *fiber.Ctx) error {
	filter, view, meta := parseMessageFilter(c)
	ctx := c.UserContext()

	msgs, err := s.store.ListMessages(ctx, filter)
	if err != nil {
		return err
	}
	total, err := s.store.CountMessages(ctx, filter)
	if err != nil {
		return err
	}

	data := fiber.Map{
		"Title":      "Messages",
		"Messages":   msgs,
		"Filter":     view,
		"Total":      total,
		"Ports":      s.cfg.SMTPPorts,
		"Actions":    models.PolicyActions,
		"Pagination": buildPagination(c, "/messages", meta.Page, meta.Limit, total),
	}
	if c.Get("HX-Request") == "true" {
		return s.renderFragment(c, "message_list", data)
	}
	return s.render(c, "messages", data)
}

func (s *Server) handleMessageDetail(c *fiber.Ctx) error {
	msg, err := s.store.GetMessage(c.UserContext(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "message not found")
	}
	return s.render(c, "message_detail", fiber.Map{
		"Title":   "Message",
		"Message": msg,
	})
}

func (s *Server) handleMessageDelete(c *fiber.Ctx) error {
	ctx := c.UserContext()
	msg, err := s.store.GetMessage(ctx, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "message not found")
	}
	if err := s.store.DeleteMessage(ctx, msg.ID); err != nil {
		return err
	}
	s.bus.Emit(events.New(events.TypeMessageDeleted, events.SeverityInfo, map[string]any{
		"message_id": msg.ID,
		"recipient":  msg.Recipient,
		"mailbox":    msg.Mailbox,
		"domain":     msg.Domain,
		"local_port": msg.LocalPort,
		"remote_ip":  msg.RemoteIP,
		"subject":    msg.Subject,
		"action":     msg.Action,
		"reason":     "manual deletion",
		"username":   c.Locals("user"),
	}))
	return c.Redirect("/messages?flash="+url.QueryEscape("Message deleted."), fiber.StatusFound)
}

func (s *Server) handleEvents(c *fiber.Ctx) error {
	filter, view, meta := parseEventFilter(c)
	ctx := c.UserContext()

	evs, err := s.store.ListEvents(ctx, filter)
	if err != nil {
		return err
	}
	total, err := s.store.CountEvents(ctx, filter)
	if err != nil {
		return err
	}
	return s.render(c, "events", fiber.Map{
		"Title":      "Event stream",
		"Events":     evs,
		"Filter":     view,
		"EventTypes": eventTypes,
		"Severities": severities,
		"Pagination": buildPagination(c, "/events", meta.Page, meta.Limit, total),
	})
}

func (s *Server) handlePolicies(c *fiber.Ctx) error {
	policies, err := s.store.ListPolicies(c.UserContext())
	if err != nil {
		return err
	}
	return s.render(c, "policies", fiber.Map{
		"Title":      "Policies",
		"Policies":   policies,
		"PolicyForm": policyFormFromModel(nil),
		"Fields":     models.PolicyFields,
		"Actions":    models.PolicyActions,
	})
}

func (s *Server) handlePolicyCreate(c *fiber.Ctx) error {
	p, err := parsePolicyForm(c)
	if err != nil {
		return c.Redirect("/policies?error="+url.QueryEscape(err.Error()), fiber.StatusFound)
	}
	if err := s.store.SavePolicy(c.UserContext(), p); err != nil {
		return err
	}
	s.bus.Emit(events.New(events.TypePolicyCreated, events.SeverityInfo, policyPayload(p, c)))
	return c.Redirect("/policies?flash="+url.QueryEscape("Policy created."), fiber.StatusFound)
}

func (s *Server) handlePolicyEditForm(c *fiber.Ctx) error {
	p, err := s.store.GetPolicy(c.UserContext(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "policy not found")
	}
	return s.renderFragment(c, "policy_form", fiber.Map{
		"PolicyForm": policyFormFromModel(p),
		"Fields":     models.PolicyFields,
		"Actions":    models.PolicyActions,
	})
}

func (s *Server) handlePolicyUpdate(c *fiber.Ctx) error {
	existing, err := s.store.GetPolicy(c.UserContext(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "policy not found")
	}
	updated, err := parsePolicyForm(c)
	if err != nil {
		return c.Redirect("/policies?error="+url.QueryEscape(err.Error()), fiber.StatusFound)
	}
	updated.ID = existing.ID
	updated.CreatedAt = existing.CreatedAt
	updated.Priority = existing.Priority
	if err := s.store.SavePolicy(c.UserContext(), updated); err != nil {
		return err
	}
	s.bus.Emit(events.New(events.TypePolicyUpdated, events.SeverityInfo, policyPayload(updated, c)))
	return c.Redirect("/policies?flash="+url.QueryEscape("Policy updated."), fiber.StatusFound)
}

func (s *Server) handlePolicyToggle(c *fiber.Ctx) error {
	p, err := s.store.GetPolicy(c.UserContext(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "policy not found")
	}
	p.Enabled = !p.Enabled
	if err := s.store.SavePolicy(c.UserContext(), p); err != nil {
		return err
	}
	s.bus.Emit(events.New(events.TypePolicyUpdated, events.SeverityInfo, policyPayload(p, c)))
	return c.Redirect("/policies?flash="+url.QueryEscape("Policy state updated."), fiber.StatusFound)
}

func (s *Server) handlePolicyMove(c *fiber.Ctx) error {
	ctx := c.UserContext()
	policies, err := s.store.ListPolicies(ctx)
	if err != nil {
		return err
	}
	index := -1
	for i, p := range policies {
		if p.ID == c.Params("id") {
			index = i
			break
		}
	}
	if index < 0 {
		return fiber.NewError(fiber.StatusNotFound, "policy not found")
	}
	target := index - 1
	if c.FormValue("dir") == "down" {
		target = index + 1
	}
	if target < 0 || target >= len(policies) {
		return c.Redirect("/policies", fiber.StatusFound)
	}
	policies[index], policies[target] = policies[target], policies[index]

	ids := make([]string, len(policies))
	for i, p := range policies {
		ids[i] = p.ID
	}
	if err := s.store.ReorderPolicies(ctx, ids); err != nil {
		return err
	}
	s.bus.Emit(events.New(events.TypePolicyReordered, events.SeverityInfo, map[string]any{
		"policy_id": c.Params("id"),
		"direction": c.FormValue("dir"),
		"username":  c.Locals("user"),
	}))
	return c.Redirect("/policies?flash="+url.QueryEscape("Policy order updated."), fiber.StatusFound)
}

func (s *Server) handlePolicyDelete(c *fiber.Ctx) error {
	p, err := s.store.GetPolicy(c.UserContext(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "policy not found")
	}
	if err := s.store.DeletePolicy(c.UserContext(), p.ID); err != nil {
		return err
	}
	s.bus.Emit(events.New(events.TypePolicyDeleted, events.SeverityInfo, policyPayload(p, c)))
	return c.Redirect("/policies?flash="+url.QueryEscape("Policy deleted."), fiber.StatusFound)
}

func (s *Server) handlePolicyTest(c *fiber.Ctx) error {
	field := c.FormValue("field")
	pattern := c.FormValue("pattern")
	value := c.FormValue("value")
	matched, err := policy.TestPattern(field, pattern, value)
	data := fiber.Map{
		"Field":   field,
		"Pattern": pattern,
		"Value":   value,
		"Matched": matched,
	}
	if err != nil {
		data["Error"] = err.Error()
	}
	return s.renderFragment(c, "policy_test", data)
}

func (s *Server) handleSettings(c *fiber.Ctx) error {
	stats, err := s.gatherStats(c.UserContext())
	if err != nil {
		return err
	}
	return s.render(c, "settings", fiber.Map{
		"Title":     "Settings",
		"Config":    s.cfg,
		"Stats":     stats,
		"Uptime":    time.Since(s.startedAt).Round(time.Second).String(),
		"StartedAt": s.startedAt,
	})
}

func policyPayload(p *models.Policy, c *fiber.Ctx) map[string]any {
	return map[string]any{
		"policy_id":   p.ID,
		"name":        p.Name,
		"field":       p.Field,
		"pattern":     p.Pattern,
		"action":      p.Action,
		"ttl_seconds": p.TTLSeconds,
		"enabled":     p.Enabled,
		"priority":    p.Priority,
		"username":    c.Locals("user"),
	}
}
