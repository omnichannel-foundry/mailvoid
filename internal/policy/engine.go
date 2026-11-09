// Package policy evaluates ordered regular-expression retention rules against
// captured mail.
package policy

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/storage"
)

const maxPatternLength = 2048

// Decision is the outcome of evaluating one message against the policy list.
type Decision struct {
	Matched     bool
	Action      string
	PolicyID    string
	PolicyName  string
	Field       string
	MatchedText string
	TTL         time.Duration
}

type compiledPattern struct {
	re  *regexp.Regexp
	err error
}

// Engine evaluates the policies in a store.
type Engine struct {
	store storage.Store
	cache sync.Map
}

// New creates a policy engine.
func New(store storage.Store) *Engine {
	return &Engine{store: store}
}

func (e *Engine) compile(pattern string) (*regexp.Regexp, error) {
	if cached, ok := e.cache.Load(pattern); ok {
		entry := cached.(compiledPattern)
		return entry.re, entry.err
	}
	re, err := regexp.Compile(pattern)
	e.cache.Store(pattern, compiledPattern{re: re, err: err})
	return re, err
}

// Evaluate returns the first enabled rule that matches the message, in priority
// order. With no match the message is kept.
func (e *Engine) Evaluate(ctx context.Context, msg *models.Message) (Decision, error) {
	policies, err := e.store.ListPolicies(ctx)
	if err != nil {
		return Decision{}, err
	}
	for _, p := range policies {
		if !p.Enabled {
			continue
		}
		re, err := e.compile(p.Pattern)
		if err != nil {
			continue
		}
		value := FieldValue(msg, p.Field)
		if !re.MatchString(value) {
			continue
		}
		return Decision{
			Matched:     true,
			Action:      p.Action,
			PolicyID:    p.ID,
			PolicyName:  p.Name,
			Field:       p.Field,
			MatchedText: value,
			TTL:         p.TTL(),
		}, nil
	}
	return Decision{Action: models.ActionKeep}, nil
}

// Apply evaluates the message and mutates its action, policy metadata, and
// expiry fields.
func (e *Engine) Apply(ctx context.Context, msg *models.Message) (Decision, error) {
	decision, err := e.Evaluate(ctx, msg)
	if err != nil {
		return decision, err
	}
	if decision.Action == "" {
		decision.Action = models.ActionKeep
	}
	msg.Action = decision.Action
	msg.PolicyID = decision.PolicyID
	msg.PolicyName = decision.PolicyName
	msg.ExpiresAt = nil
	if decision.Action == models.ActionTTL && decision.TTL > 0 {
		expires := time.Now().UTC().Add(decision.TTL)
		msg.ExpiresAt = &expires
	}
	return decision, nil
}

// FieldValue extracts the text a policy field targets.
func FieldValue(msg *models.Message, field string) string {
	switch field {
	case models.FieldSender:
		return msg.Sender
	case models.FieldRecipient:
		return msg.Recipient
	case models.FieldMailbox:
		return msg.Mailbox
	case models.FieldDomain:
		return msg.Domain
	case models.FieldSubject:
		return msg.Subject
	case models.FieldRemoteIP:
		return msg.RemoteIP
	case models.FieldLocalPort:
		return strconv.Itoa(msg.LocalPort)
	case models.FieldAny:
		return strings.Join([]string{
			msg.Sender, msg.Recipient, msg.Mailbox, msg.Domain, msg.Subject,
			msg.RemoteIP, strconv.Itoa(msg.LocalPort),
		}, " ")
	default:
		return ""
	}
}

// Validate checks a policy before it is persisted.
func Validate(p *models.Policy) error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("policy name is required")
	}
	if !validField(p.Field) {
		return fmt.Errorf("unknown policy field %q", p.Field)
	}
	if !validAction(p.Action) {
		return fmt.Errorf("unknown policy action %q", p.Action)
	}
	if p.Pattern == "" {
		return fmt.Errorf("pattern is required")
	}
	if len(p.Pattern) > maxPatternLength {
		return fmt.Errorf("pattern exceeds %d characters", maxPatternLength)
	}
	if _, err := regexp.Compile(p.Pattern); err != nil {
		return fmt.Errorf("invalid regular expression: %w", err)
	}
	if p.Action == models.ActionTTL && p.TTLSeconds <= 0 {
		return fmt.Errorf("TTL must be greater than zero for the ttl action")
	}
	if p.Action != models.ActionTTL {
		p.TTLSeconds = 0
	}
	return nil
}

// TestPattern reports whether pattern matches value for the given field.
func TestPattern(field, pattern, value string) (bool, error) {
	if !validField(field) {
		return false, fmt.Errorf("unknown policy field %q", field)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("invalid regular expression: %w", err)
	}
	return re.MatchString(value), nil
}

func validField(field string) bool {
	for _, f := range models.PolicyFields {
		if f == field {
			return true
		}
	}
	return false
}

func validAction(action string) bool {
	for _, a := range models.PolicyActions {
		if a == action {
			return true
		}
	}
	return false
}
