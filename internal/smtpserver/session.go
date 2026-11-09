package smtpserver

import (
	"context"
	"errors"
	"io"
	"net/mail"
	"strings"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/mailparse"
	"github.com/gnrfan/mailvoid/internal/models"
)

type session struct {
	backend    *Backend
	remoteIP   string
	localPort  int
	from       string
	recipients []string
}

func (s *session) Mail(from string, _ *smtp.MailOptions) error {
	mailbox, domain, canonical, err := splitAddress(from)
	if err != nil {
		return &smtp.SMTPError{
			Code:         501,
			EnhancedCode: smtp.EnhancedCode{5, 1, 7},
			Message:      "invalid sender address",
		}
	}
	_ = mailbox
	_ = domain
	s.from = canonical

	s.backend.bus.Emit(events.New(events.TypeSMTPMailFrom, events.SeverityInfo, map[string]any{
		"remote_ip":  s.remoteIP,
		"local_port": s.localPort,
		"sender":     canonical,
	}))
	return nil
}

func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	mailbox, domain, canonical, err := splitAddress(to)
	if err != nil || mailbox == "" || domain == "" {
		s.backend.bus.Emit(events.New(events.TypeSMTPRcptTo, events.SeverityWarn, map[string]any{
			"remote_ip":  s.remoteIP,
			"local_port": s.localPort,
			"recipient":  to,
			"accepted":   false,
			"reason":     "invalid address",
		}))
		return &smtp.SMTPError{
			Code:         501,
			EnhancedCode: smtp.EnhancedCode{5, 1, 3},
			Message:      "invalid recipient address",
		}
	}

	if !s.backend.cfg.AllowsDomain(domain) {
		s.backend.bus.Emit(events.New(events.TypeSMTPRcptTo, events.SeverityWarn, map[string]any{
			"remote_ip":  s.remoteIP,
			"local_port": s.localPort,
			"recipient":  canonical,
			"domain":     domain,
			"accepted":   false,
			"reason":     "domain not accepted",
		}))
		return &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 1, 1},
			Message:      "relay access denied",
		}
	}

	s.recipients = append(s.recipients, canonical)
	s.backend.bus.Emit(events.New(events.TypeSMTPRcptTo, events.SeverityInfo, map[string]any{
		"remote_ip":  s.remoteIP,
		"local_port": s.localPort,
		"recipient":  canonical,
		"domain":     domain,
		"mailbox":    mailbox,
		"accepted":   true,
	}))
	return nil
}

func (s *session) Data(r io.Reader) error {
	if len(s.recipients) == 0 {
		return &smtp.SMTPError{
			Code:         503,
			EnhancedCode: smtp.EnhancedCode{5, 5, 1},
			Message:      "no recipients",
		}
	}

	max := s.backend.cfg.MaxMessageBytes
	if max <= 0 {
		max = 25 << 20
	}
	raw, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return &smtp.SMTPError{
			Code:         451,
			EnhancedCode: smtp.EnhancedCode{4, 3, 0},
			Message:      "failed to read message",
		}
	}
	if int64(len(raw)) > max {
		s.backend.bus.Emit(events.New(events.TypeSMTPError, events.SeverityWarn, map[string]any{
			"remote_ip":  s.remoteIP,
			"local_port": s.localPort,
			"error":      "message too large",
			"size":       len(raw),
		}))
		return &smtp.SMTPError{
			Code:         552,
			EnhancedCode: smtp.EnhancedCode{5, 3, 4},
			Message:      "message too large",
		}
	}

	s.backend.bus.Emit(events.New(events.TypeSMTPData, events.SeverityInfo, map[string]any{
		"remote_ip":  s.remoteIP,
		"local_port": s.localPort,
		"size":       len(raw),
		"recipients": len(s.recipients),
	}))

	parsed, parseErr := mailparse.Parse(raw)
	if parseErr != nil {
		s.backend.bus.Emit(events.New(events.TypeSMTPError, events.SeverityWarn, map[string]any{
			"remote_ip":  s.remoteIP,
			"local_port": s.localPort,
			"error":      parseErr.Error(),
			"stored":     true,
		}))
		parsed = &mailparse.Parsed{
			Header:  map[string][]string{},
			Subject: "(unparsable message)",
		}
	}

	now := time.Now().UTC()
	ctx := context.Background()
	for _, recipient := range s.recipients {
		mailbox, domain, canonical, err := splitAddress(recipient)
		if err != nil || domain == "" {
			continue
		}
		msg := &models.Message{
			ID:         models.NewID("msg"),
			ReceivedAt: now,
			RemoteIP:   s.remoteIP,
			LocalPort:  s.localPort,
			Domain:     domain,
			Mailbox:    mailbox,
			Recipient:  canonical,
			Sender:     s.from,
			Subject:    parsed.Subject,
			Headers:    parsed.Header,
			RawHeaders: parsed.RawHeaders,
			Raw:        string(raw),
			BodyText:   parsed.Text,
			BodyHTML:   parsed.HTML,
			Size:       int64(len(raw)),
			Action:     models.ActionKeep,
		}

		decision, policyErr := s.backend.engine.Apply(ctx, msg)
		if policyErr != nil {
			s.backend.bus.Emit(events.New(events.TypeSMTPError, events.SeverityError, map[string]any{
				"remote_ip":  s.remoteIP,
				"local_port": s.localPort,
				"recipient":  canonical,
				"error":      policyErr.Error(),
			}))
			msg.Action = models.ActionKeep
			msg.PolicyID = ""
			msg.PolicyName = ""
			msg.ExpiresAt = nil
		} else if decision.Matched {
			s.backend.bus.Emit(events.New(events.TypePolicyMatched, events.SeverityInfo, map[string]any{
				"remote_ip":   s.remoteIP,
				"local_port":  s.localPort,
				"recipient":   canonical,
				"policy_id":   decision.PolicyID,
				"policy_name": decision.PolicyName,
				"field":       decision.Field,
				"action":      decision.Action,
			}))
		}

		if msg.Action == models.ActionDrop {
			s.backend.bus.Emit(events.New(events.TypeMessageDropped, events.SeverityWarn, messagePayload(msg, "policy drop")))
			continue
		}

		if err := s.backend.store.SaveMessage(ctx, msg); err != nil {
			s.backend.bus.Emit(events.New(events.TypeMessageError, events.SeverityError, map[string]any{
				"remote_ip":  s.remoteIP,
				"local_port": s.localPort,
				"recipient":  canonical,
				"error":      err.Error(),
			}))
			return &smtp.SMTPError{
				Code:         451,
				EnhancedCode: smtp.EnhancedCode{4, 3, 0},
				Message:      "temporary storage failure",
			}
		}

		s.backend.bus.Emit(events.New(events.TypeMessageReceived, events.SeverityInfo, messagePayload(msg, "")))
	}

	s.Reset()
	return nil
}

func messagePayload(msg *models.Message, reason string) map[string]any {
	payload := map[string]any{
		"message_id": msg.ID,
		"remote_ip":  msg.RemoteIP,
		"local_port": msg.LocalPort,
		"domain":     msg.Domain,
		"mailbox":    msg.Mailbox,
		"recipient":  msg.Recipient,
		"sender":     msg.Sender,
		"subject":    msg.Subject,
		"size":       msg.Size,
		"action":     msg.Action,
		"policy_id":  msg.PolicyID,
		"policy":     msg.PolicyName,
	}
	if msg.ExpiresAt != nil {
		payload["expires_at"] = msg.ExpiresAt.Format(time.RFC3339)
	}
	if reason != "" {
		payload["reason"] = reason
	}
	return payload
}

func (s *session) Reset() {
	s.from = ""
	s.recipients = nil
}

func (s *session) Logout() error {
	s.backend.bus.Emit(events.New(events.TypeSMTPClosed, events.SeverityInfo, map[string]any{
		"remote_ip":  s.remoteIP,
		"local_port": s.localPort,
	}))
	return nil
}

func splitAddress(raw string) (mailbox, domain, canonical string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "<>" {
		return "", "", "", nil
	}
	if parsed, parseErr := mail.ParseAddress(raw); parseErr == nil {
		raw = parsed.Address
	}
	raw = strings.TrimPrefix(raw, "<")
	raw = strings.TrimSuffix(raw, ">")
	at := strings.LastIndex(raw, "@")
	if at <= 0 || at == len(raw)-1 {
		return "", "", raw, errors.New("invalid address")
	}
	mailbox = strings.ToLower(strings.TrimSpace(raw[:at]))
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw[at+1:]), "."))
	canonical = mailbox + "@" + domain
	return mailbox, domain, canonical, nil
}
