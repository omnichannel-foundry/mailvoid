// Package mailbox provides mailbox-centric views over captured messages. It is
// shared by the versioned REST API and the MCP tool surface.
package mailbox

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/storage"
)

// ErrInvalidAddress is returned for addresses that do not contain a mailbox and
// a domain.
var ErrInvalidAddress = errors.New("invalid mailbox address")

// ErrTimeout is returned when Wait does not observe a new message in time.
var ErrTimeout = errors.New("timed out waiting for mail")

const (
	scanPageSize = 500
	maxScanPages = 10000
)

// Summary describes one mailbox.
type Summary struct {
	Address          string     `json:"address"`
	Mailbox          string     `json:"mailbox"`
	Domain           string     `json:"domain"`
	MessageCount     int        `json:"message_count"`
	LatestMessageID  string     `json:"latest_message_id,omitempty"`
	LatestSubject    string     `json:"latest_subject,omitempty"`
	LatestSender     string     `json:"latest_sender,omitempty"`
	LatestReceivedAt *time.Time `json:"latest_received_at,omitempty"`
}

// ListOptions filters the mailbox directory.
type ListOptions struct {
	Domain string
	Since  time.Time
	Limit  int
	Offset int
}

// MessageOptions filters messages within a mailbox.
type MessageOptions struct {
	Query  string
	Action string
	Port   int
	Since  time.Time
	Until  time.Time
	Limit  int
	Offset int
}

// Service builds mailbox views from a Store.
type Service struct {
	store storage.Store
}

// New creates a mailbox service.
func New(store storage.Store) *Service {
	return &Service{store: store}
}

// ParseAddress splits and validates a mailbox@domain address.
func ParseAddress(address string) (mailbox, domain string, err error) {
	address = strings.TrimSpace(address)
	if address == "" || strings.ContainsAny(address, " \t\r\n") {
		return "", "", ErrInvalidAddress
	}
	mailbox, domain = models.SplitAddress(address)
	if mailbox == "" || domain == "" {
		return "", "", ErrInvalidAddress
	}
	return mailbox, domain, nil
}

// All returns every mailbox summary ordered by most recent message.
func (s *Service) All(ctx context.Context, domain string, since time.Time) ([]Summary, error) {
	summaries := make(map[string]*Summary)
	seen := make(map[string]bool)

	for page := 0; page < maxScanPages; page++ {
		messages, err := s.store.ListMessages(ctx, models.MessageFilter{
			Domain: domain,
			Since:  since,
			Limit:  scanPageSize,
			Offset: page * scanPageSize,
		})
		if err != nil {
			return nil, err
		}
		for _, msg := range messages {
			if seen[msg.ID] {
				continue
			}
			seen[msg.ID] = true
			key := msg.Address()
			summary, ok := summaries[key]
			if !ok {
				summary = &Summary{
					Address: key,
					Mailbox: msg.Mailbox,
					Domain:  msg.Domain,
				}
				summaries[key] = summary
			}
			summary.MessageCount++
			if summary.LatestReceivedAt == nil || msg.ReceivedAt.After(*summary.LatestReceivedAt) {
				received := msg.ReceivedAt
				summary.LatestReceivedAt = &received
				summary.LatestMessageID = msg.ID
				summary.LatestSubject = msg.Subject
				summary.LatestSender = msg.Sender
			}
		}
		if len(messages) < scanPageSize {
			break
		}
	}

	out := make([]Summary, 0, len(summaries))
	for _, summary := range summaries {
		out = append(out, *summary)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i].LatestReceivedAt, out[j].LatestReceivedAt
		switch {
		case left == nil && right == nil:
			return out[i].Address < out[j].Address
		case left == nil:
			return false
		case right == nil:
			return true
		case left.Equal(*right):
			return out[i].Address < out[j].Address
		default:
			return left.After(*right)
		}
	})
	return out, nil
}

// List returns a page of mailbox summaries ordered by most recent message.
func (s *Service) List(ctx context.Context, opts ListOptions) ([]Summary, int, error) {
	all, err := s.All(ctx, opts.Domain, opts.Since)
	if err != nil {
		return nil, 0, err
	}
	total := len(all)
	offset, limit := normalize(opts.Offset, opts.Limit)
	if offset >= total {
		return []Summary{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

// Summary returns the aggregate view for one mailbox.
func (s *Service) Summary(ctx context.Context, address string, since time.Time) (Summary, error) {
	mailbox, domain, err := ParseAddress(address)
	if err != nil {
		return Summary{}, err
	}
	filter := models.MessageFilter{Mailbox: mailbox, Domain: domain, Since: since}
	count, err := s.store.CountMessages(ctx, filter)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{
		Address:      mailbox + "@" + domain,
		Mailbox:      mailbox,
		Domain:       domain,
		MessageCount: count,
	}
	filter.Limit = 1
	messages, err := s.store.ListMessages(ctx, filter)
	if err != nil {
		return Summary{}, err
	}
	if len(messages) > 0 {
		msg := messages[0]
		received := msg.ReceivedAt
		summary.LatestReceivedAt = &received
		summary.LatestMessageID = msg.ID
		summary.LatestSubject = msg.Subject
		summary.LatestSender = msg.Sender
	}
	return summary, nil
}

// Messages returns a page of messages for one mailbox and the total count.
func (s *Service) Messages(ctx context.Context, address string, opts MessageOptions) ([]*models.Message, int, error) {
	mailbox, domain, err := ParseAddress(address)
	if err != nil {
		return nil, 0, err
	}
	offset, limit := normalize(opts.Offset, opts.Limit)
	filter := models.MessageFilter{
		Query:   opts.Query,
		Mailbox: mailbox,
		Domain:  domain,
		Action:  opts.Action,
		Port:    opts.Port,
		Since:   opts.Since,
		Until:   opts.Until,
		Limit:   limit,
		Offset:  offset,
	}
	messages, err := s.store.ListMessages(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountMessages(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	return messages, total, nil
}

// AllMessages returns every message for one mailbox, newest first.
func (s *Service) AllMessages(ctx context.Context, address string, opts MessageOptions) ([]*models.Message, error) {
	mailboxName, domain, err := ParseAddress(address)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var out []*models.Message
	for page := 0; page < maxScanPages; page++ {
		messages, err := s.store.ListMessages(ctx, models.MessageFilter{
			Query:   opts.Query,
			Mailbox: mailboxName,
			Domain:  domain,
			Action:  opts.Action,
			Port:    opts.Port,
			Since:   opts.Since,
			Until:   opts.Until,
			Limit:   scanPageSize,
			Offset:  page * scanPageSize,
		})
		if err != nil {
			return nil, err
		}
		for _, message := range messages {
			if seen[message.ID] {
				continue
			}
			seen[message.ID] = true
			out = append(out, message)
		}
		if len(messages) < scanPageSize {
			break
		}
	}
	return out, nil
}

// Wait polls until a message newer than since arrives for the mailbox, or the
// timeout/context expires.
func (s *Service) Wait(ctx context.Context, address string, since time.Time, timeout time.Duration) (*models.Message, error) {
	mailbox, domain, err := ParseAddress(address)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}
	if since.IsZero() {
		since = time.Now().UTC()
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	check := func() (*models.Message, error) {
		messages, err := s.store.ListMessages(waitCtx, models.MessageFilter{
			Mailbox: mailbox,
			Domain:  domain,
			Since:   since,
			Limit:   1,
		})
		if err != nil {
			return nil, err
		}
		if len(messages) == 0 {
			return nil, nil
		}
		if !messages[0].ReceivedAt.After(since) {
			return nil, nil
		}
		return messages[0], nil
	}

	msg, err := check()
	if err != nil || msg != nil {
		return msg, err
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) || errors.Is(waitCtx.Err(), context.Canceled) {
				return nil, ErrTimeout
			}
			return nil, waitCtx.Err()
		case <-ticker.C:
			msg, err := check()
			if err != nil {
				return nil, err
			}
			if msg != nil {
				return msg, nil
			}
		}
	}
}

func normalize(offset, limit int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	return offset, limit
}

// String implements fmt.Stringer for diagnostics.
func (s Summary) String() string {
	return fmt.Sprintf("%s (%d)", s.Address, s.MessageCount)
}
