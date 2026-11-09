package web

import (
	"bufio"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/policy"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

type apiMessageSummary struct {
	ID         string     `json:"id"`
	ReceivedAt time.Time  `json:"received_at"`
	RemoteIP   string     `json:"remote_ip"`
	LocalPort  int        `json:"local_port"`
	Domain     string     `json:"domain"`
	Mailbox    string     `json:"mailbox"`
	Recipient  string     `json:"recipient"`
	Sender     string     `json:"sender"`
	Subject    string     `json:"subject"`
	Size       int64      `json:"size"`
	Action     string     `json:"action"`
	PolicyID   string     `json:"policy_id,omitempty"`
	PolicyName string     `json:"policy_name,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

func summarizeMessage(m *models.Message) apiMessageSummary {
	return apiMessageSummary{
		ID:         m.ID,
		ReceivedAt: m.ReceivedAt,
		RemoteIP:   m.RemoteIP,
		LocalPort:  m.LocalPort,
		Domain:     m.Domain,
		Mailbox:    m.Mailbox,
		Recipient:  m.Recipient,
		Sender:     m.Sender,
		Subject:    m.Subject,
		Size:       m.Size,
		Action:     m.Action,
		PolicyID:   m.PolicyID,
		PolicyName: m.PolicyName,
		ExpiresAt:  m.ExpiresAt,
	}
}

func (s *Server) apiListMessages(c *fiber.Ctx) error {
	filter, _, meta := parseMessageFilter(c)
	msgs, err := s.store.ListMessages(c.UserContext(), filter)
	if err != nil {
		return err
	}
	total, err := s.store.CountMessages(c.UserContext(), filter)
	if err != nil {
		return err
	}
	out := make([]apiMessageSummary, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, summarizeMessage(m))
	}
	return c.JSON(fiber.Map{
		"messages": out,
		"count":    total,
		"limit":    meta.Limit,
		"offset":   filter.Offset,
	})
}

func (s *Server) apiGetMessage(c *fiber.Ctx) error {
	msg, err := s.store.GetMessage(c.UserContext(), c.Params("id"))
	if err != nil {
		return apiError(c, fiber.StatusNotFound, "message not found")
	}
	return c.JSON(msg)
}

func (s *Server) apiDeleteMessage(c *fiber.Ctx) error {
	ctx := c.UserContext()
	msg, err := s.store.GetMessage(ctx, c.Params("id"))
	if err != nil {
		return apiError(c, fiber.StatusNotFound, "message not found")
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
		"reason":     "api deletion",
		"username":   c.Locals("user"),
	}))
	return c.JSON(fiber.Map{"deleted": true, "id": msg.ID})
}

func (s *Server) apiListEvents(c *fiber.Ctx) error {
	filter, _, meta := parseEventFilter(c)
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
		"limit":  meta.Limit,
		"offset": filter.Offset,
	})
}

func (s *Server) apiListPolicies(c *fiber.Ctx) error {
	policies, err := s.store.ListPolicies(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"policies": policies, "count": len(policies)})
}

func (s *Server) apiGetPolicy(c *fiber.Ctx) error {
	p, err := s.store.GetPolicy(c.UserContext(), c.Params("id"))
	if err != nil {
		return apiError(c, fiber.StatusNotFound, "policy not found")
	}
	return c.JSON(p)
}

type policyInput struct {
	Name       string `json:"name"`
	Field      string `json:"field"`
	Pattern    string `json:"pattern"`
	Action     string `json:"action"`
	TTLSeconds int64  `json:"ttl_seconds"`
	Priority   *int   `json:"priority"`
	Enabled    *bool  `json:"enabled"`
}

func (in policyInput) apply(p *models.Policy) *models.Policy {
	p.Name = in.Name
	p.Field = in.Field
	p.Pattern = in.Pattern
	p.Action = in.Action
	p.TTLSeconds = in.TTLSeconds
	if in.Priority != nil {
		p.Priority = *in.Priority
	}
	if in.Enabled != nil {
		p.Enabled = *in.Enabled
	}
	return p
}

func (s *Server) apiCreatePolicy(c *fiber.Ctx) error {
	var in policyInput
	if err := c.BodyParser(&in); err != nil {
		return apiError(c, fiber.StatusBadRequest, "invalid JSON body")
	}
	p := in.apply(&models.Policy{Enabled: true})
	if in.Enabled == nil {
		p.Enabled = true
	}
	if err := policy.Validate(p); err != nil {
		return apiError(c, fiber.StatusBadRequest, err.Error())
	}
	if err := s.store.SavePolicy(c.UserContext(), p); err != nil {
		return err
	}
	s.bus.Emit(events.New(events.TypePolicyCreated, events.SeverityInfo, policyPayload(p, c)))
	return c.Status(fiber.StatusCreated).JSON(p)
}

func (s *Server) apiUpdatePolicy(c *fiber.Ctx) error {
	existing, err := s.store.GetPolicy(c.UserContext(), c.Params("id"))
	if err != nil {
		return apiError(c, fiber.StatusNotFound, "policy not found")
	}
	var in policyInput
	if err := c.BodyParser(&in); err != nil {
		return apiError(c, fiber.StatusBadRequest, "invalid JSON body")
	}
	updated := in.apply(existing)
	if err := policy.Validate(updated); err != nil {
		return apiError(c, fiber.StatusBadRequest, err.Error())
	}
	if err := s.store.SavePolicy(c.UserContext(), updated); err != nil {
		return err
	}
	s.bus.Emit(events.New(events.TypePolicyUpdated, events.SeverityInfo, policyPayload(updated, c)))
	return c.JSON(updated)
}

func (s *Server) apiDeletePolicy(c *fiber.Ctx) error {
	ctx := c.UserContext()
	p, err := s.store.GetPolicy(ctx, c.Params("id"))
	if err != nil {
		return apiError(c, fiber.StatusNotFound, "policy not found")
	}
	if err := s.store.DeletePolicy(ctx, p.ID); err != nil {
		return err
	}
	s.bus.Emit(events.New(events.TypePolicyDeleted, events.SeverityInfo, policyPayload(p, c)))
	return c.JSON(fiber.Map{"deleted": true, "id": p.ID})
}

func (s *Server) apiReorderPolicies(c *fiber.Ctx) error {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := c.BodyParser(&body); err != nil || len(body.IDs) == 0 {
		return apiError(c, fiber.StatusBadRequest, "expected {\"ids\": [...]}")
	}
	existing, err := s.store.ListPolicies(c.UserContext())
	if err != nil {
		return err
	}
	if len(existing) != len(body.IDs) {
		return apiError(c, fiber.StatusBadRequest, "ids must include every policy exactly once")
	}
	seen := make(map[string]bool, len(existing))
	for _, p := range existing {
		seen[p.ID] = true
	}
	for _, id := range body.IDs {
		if !seen[id] {
			return apiError(c, fiber.StatusBadRequest, "ids must include every policy exactly once")
		}
		delete(seen, id)
	}
	if len(seen) != 0 {
		return apiError(c, fiber.StatusBadRequest, "ids must include every policy exactly once")
	}
	if err := s.store.ReorderPolicies(c.UserContext(), body.IDs); err != nil {
		return err
	}
	s.bus.Emit(events.New(events.TypePolicyReordered, events.SeverityInfo, map[string]any{
		"ids":      body.IDs,
		"username": c.Locals("user"),
	}))
	return c.JSON(fiber.Map{"reordered": true})
}

func (s *Server) apiTestPolicy(c *fiber.Ctx) error {
	var body struct {
		Field   string `json:"field"`
		Pattern string `json:"pattern"`
		Value   string `json:"value"`
	}
	if err := c.BodyParser(&body); err != nil {
		return apiError(c, fiber.StatusBadRequest, "invalid JSON body")
	}
	matched, err := policy.TestPattern(body.Field, body.Pattern, body.Value)
	if err != nil {
		return apiError(c, fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{
		"matched": matched,
		"field":   body.Field,
		"pattern": body.Pattern,
		"value":   body.Value,
	})
}

func (s *Server) apiStats(c *fiber.Ctx) error {
	stats, err := s.gatherStats(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"messages":     stats.TotalMessages,
		"messages_24h": stats.Last24h,
		"policies":     stats.Policies,
		"events":       stats.Events,
		"version":      s.version,
		"uptime":       time.Since(s.startedAt).Round(time.Second).String(),
	})
}

func (s *Server) apiEventStream(c *fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	ch, unsubscribe := s.bus.Subscribe(128)
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer unsubscribe()
		_, _ = fmt.Fprint(w, "event: ready\ndata: {\"ok\":true}\n\n")
		if err := w.Flush(); err != nil {
			return
		}
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return
				}
				payload, err := json.Marshal(ev)
				if err != nil {
					continue
				}
				if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			case <-ticker.C:
				if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			case <-s.done:
				return
			}
		}
	})
	return nil
}

func (s *Server) handleWebSocket(conn *websocket.Conn) {
	defer conn.Close()
	ch, unsubscribe := s.bus.Subscribe(128)
	defer unsubscribe()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteJSON(ev); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		case <-s.done:
			return
		}
	}
}
