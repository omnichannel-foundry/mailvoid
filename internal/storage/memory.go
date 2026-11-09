package storage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
)

// Memory is a thread-safe in-memory Store used by tests and by deployments that
// prefer ephemeral capture.
type Memory struct {
	mu          sync.RWMutex
	messages    map[string]models.Message
	policies    map[string]models.Policy
	events      []events.Event
	users       map[string]models.User
	apiKeys     map[string]models.APIKey
	webdavUsers map[string]models.WebDAVUser
}

// NewMemory creates an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{
		messages:    make(map[string]models.Message),
		policies:    make(map[string]models.Policy),
		users:       make(map[string]models.User),
		apiKeys:     make(map[string]models.APIKey),
		webdavUsers: make(map[string]models.WebDAVUser),
	}
}

func cloneMessage(m models.Message) models.Message {
	out := m
	if m.Headers != nil {
		out.Headers = make(map[string][]string, len(m.Headers))
		for k, v := range m.Headers {
			out.Headers[k] = append([]string(nil), v...)
		}
	}
	if m.ExpiresAt != nil {
		t := *m.ExpiresAt
		out.ExpiresAt = &t
	}
	if m.DeletedAt != nil {
		t := *m.DeletedAt
		out.DeletedAt = &t
	}
	return out
}

// SaveMessage inserts or replaces a message.
func (s *Memory) SaveMessage(_ context.Context, m *models.Message) error {
	if m.ID == "" {
		m.ID = models.NewID("msg")
	}
	normalizeMessage(m)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[m.ID] = cloneMessage(*m)
	return nil
}

// GetMessage returns a message by id.
func (s *Memory) GetMessage(_ context.Context, id string) (*models.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.messages[id]
	if !ok {
		return nil, ErrNotFound
	}
	out := cloneMessage(m)
	return &out, nil
}

func matchMessage(m models.Message, f models.MessageFilter) bool {
	if !f.IncludeDeleted && m.Deleted {
		return false
	}
	if f.Query != "" && !strings.Contains(m.SearchText(), strings.ToLower(f.Query)) {
		return false
	}
	if f.Domain != "" && !strings.EqualFold(m.Domain, f.Domain) {
		return false
	}
	if f.Mailbox != "" && !strings.EqualFold(m.Mailbox, f.Mailbox) {
		return false
	}
	if f.Action != "" && m.Action != f.Action {
		return false
	}
	if f.Port != 0 && m.LocalPort != f.Port {
		return false
	}
	if !f.Since.IsZero() && m.ReceivedAt.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && m.ReceivedAt.After(f.Until) {
		return false
	}
	return true
}

// ListMessages returns matching messages, newest first.
func (s *Memory) ListMessages(_ context.Context, f models.MessageFilter) ([]*models.Message, error) {
	s.mu.RLock()
	out := make([]*models.Message, 0, len(s.messages))
	for _, m := range s.messages {
		if matchMessage(m, f) {
			c := cloneMessage(m)
			out = append(out, &c)
		}
	}
	s.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		return out[i].ReceivedAt.After(out[j].ReceivedAt)
	})
	offset, limit := normalizeOffset(f.Offset), normalizeLimit(f.Limit)
	if offset >= len(out) {
		return []*models.Message{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

// CountMessages returns the number of messages matching f.
func (s *Memory) CountMessages(_ context.Context, f models.MessageFilter) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, m := range s.messages {
		if matchMessage(m, f) {
			n++
		}
	}
	return n, nil
}

// DeleteMessage removes a message by id.
func (s *Memory) DeleteMessage(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.messages[id]; !ok {
		return ErrNotFound
	}
	delete(s.messages, id)
	return nil
}

// DeleteExpired removes and returns messages whose expiry is at or before now.
func (s *Memory) DeleteExpired(_ context.Context, now time.Time) ([]*models.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var expired []*models.Message
	for id, m := range s.messages {
		if m.ExpiresAt != nil && !m.ExpiresAt.After(now) {
			c := cloneMessage(m)
			expired = append(expired, &c)
			delete(s.messages, id)
		}
	}
	return expired, nil
}

// SavePolicy inserts or updates a policy.
func (s *Memory) SavePolicy(_ context.Context, p *models.Policy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if p.ID == "" {
		p.ID = models.NewID("pol")
	}
	existing, exists := s.policies[p.ID]
	if exists {
		p.CreatedAt = existing.CreatedAt
	} else {
		if p.CreatedAt.IsZero() {
			p.CreatedAt = now
		}
		if p.Priority == 0 && len(s.policies) > 0 {
			max := 0
			for _, current := range s.policies {
				if current.Priority > max {
					max = current.Priority
				}
			}
			p.Priority = max + 1
		}
	}
	p.UpdatedAt = now
	s.policies[p.ID] = *p
	return nil
}

// GetPolicy returns a policy by id.
func (s *Memory) GetPolicy(_ context.Context, id string) (*models.Policy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.policies[id]
	if !ok {
		return nil, ErrNotFound
	}
	out := p
	return &out, nil
}

// ListPolicies returns policies ordered by ascending priority.
func (s *Memory) ListPolicies(_ context.Context) ([]*models.Policy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*models.Policy, 0, len(s.policies))
	for _, p := range s.policies {
		c := p
		out = append(out, &c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority == out[j].Priority {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Priority < out[j].Priority
	})
	return out, nil
}

// DeletePolicy removes a policy by id.
func (s *Memory) DeletePolicy(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.policies[id]; !ok {
		return ErrNotFound
	}
	delete(s.policies, id)
	return nil
}

// ReorderPolicies assigns priorities according to the provided id order.
func (s *Memory) ReorderPolicies(_ context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, id := range ids {
		p, ok := s.policies[id]
		if !ok {
			return ErrNotFound
		}
		p.Priority = i
		p.UpdatedAt = time.Now().UTC()
		s.policies[id] = p
	}
	return nil
}

// SaveEvent appends an event to the in-memory history.
func (s *Memory) SaveEvent(_ context.Context, ev events.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	return nil
}

// Write implements events.Sink.
func (s *Memory) Write(ev events.Event) error {
	return s.SaveEvent(context.Background(), ev)
}

// ListEvents returns matching events, newest first.
func (s *Memory) ListEvents(_ context.Context, f models.EventFilter) ([]events.Event, error) {
	s.mu.RLock()
	var out []events.Event
	for _, ev := range s.events {
		if matchEvent(ev, f) {
			out = append(out, ev)
		}
	}
	s.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})
	offset, limit := normalizeOffset(f.Offset), normalizeLimit(f.Limit)
	if offset >= len(out) {
		return []events.Event{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

// CountEvents returns the number of events matching f.
func (s *Memory) CountEvents(_ context.Context, f models.EventFilter) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, ev := range s.events {
		if matchEvent(ev, f) {
			n++
		}
	}
	return n, nil
}

func matchEvent(ev events.Event, f models.EventFilter) bool {
	if f.Type != "" && ev.Type != f.Type {
		return false
	}
	if f.Severity != "" && ev.Severity != f.Severity {
		return false
	}
	if !f.Since.IsZero() && ev.Timestamp.Before(f.Since) {
		return false
	}
	if f.Query != "" && !eventMatches(ev, f.Query) {
		return false
	}
	return true
}

// GetUser returns a user by username.
func (s *Memory) GetUser(_ context.Context, username string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[strings.ToLower(username)]
	if !ok {
		return nil, ErrNotFound
	}
	out := u
	return &out, nil
}

// SaveUser inserts or replaces a user.
func (s *Memory) SaveUser(_ context.Context, u *models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.ToLower(u.Username)
	u.Username = key
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	s.users[key] = *u
	return nil
}

// SaveAPIKey inserts or replaces an API key.
func (s *Memory) SaveAPIKey(_ context.Context, key *models.APIKey) error {
	if key.ID == "" {
		key.ID = models.NewID("key")
	}
	if key.CreatedAt.IsZero() {
		key.CreatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := *key
	if key.ExpiresAt != nil {
		expires := *key.ExpiresAt
		out.ExpiresAt = &expires
	}
	s.apiKeys[key.ID] = out
	return nil
}

// GetAPIKey returns an API key by id.
func (s *Memory) GetAPIKey(_ context.Context, id string) (*models.APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.apiKeys[id]
	if !ok {
		return nil, ErrNotFound
	}
	out := key
	if key.ExpiresAt != nil {
		expires := *key.ExpiresAt
		out.ExpiresAt = &expires
	}
	return &out, nil
}

// GetAPIKeyByPrefix returns an API key by its public prefix.
func (s *Memory) GetAPIKeyByPrefix(_ context.Context, prefix string) (*models.APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, key := range s.apiKeys {
		if key.Prefix == prefix {
			out := key
			if key.ExpiresAt != nil {
				expires := *key.ExpiresAt
				out.ExpiresAt = &expires
			}
			return &out, nil
		}
	}
	return nil, ErrNotFound
}

// ListAPIKeys returns API keys newest first.
func (s *Memory) ListAPIKeys(_ context.Context) ([]*models.APIKey, error) {
	s.mu.RLock()
	out := make([]*models.APIKey, 0, len(s.apiKeys))
	for _, key := range s.apiKeys {
		copyKey := key
		if key.ExpiresAt != nil {
			expires := *key.ExpiresAt
			copyKey.ExpiresAt = &expires
		}
		out = append(out, &copyKey)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// DeleteAPIKey removes an API key by id.
func (s *Memory) DeleteAPIKey(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.apiKeys[id]; !ok {
		return ErrNotFound
	}
	delete(s.apiKeys, id)
	return nil
}

// SaveWebDAVUser inserts or replaces a WebDAV credential.
func (s *Memory) SaveWebDAVUser(_ context.Context, user *models.WebDAVUser) error {
	user.Username = strings.ToLower(strings.TrimSpace(user.Username))
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.webdavUsers[user.Username] = *user
	return nil
}

// GetWebDAVUser returns a WebDAV credential by username.
func (s *Memory) GetWebDAVUser(_ context.Context, username string) (*models.WebDAVUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.webdavUsers[strings.ToLower(strings.TrimSpace(username))]
	if !ok {
		return nil, ErrNotFound
	}
	out := user
	return &out, nil
}

// ListWebDAVUsers returns WebDAV credentials ordered by username.
func (s *Memory) ListWebDAVUsers(_ context.Context) ([]*models.WebDAVUser, error) {
	s.mu.RLock()
	out := make([]*models.WebDAVUser, 0, len(s.webdavUsers))
	for _, user := range s.webdavUsers {
		copyUser := user
		out = append(out, &copyUser)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out, nil
}

// DeleteWebDAVUser removes a WebDAV credential.
func (s *Memory) DeleteWebDAVUser(_ context.Context, username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.ToLower(strings.TrimSpace(username))
	if _, ok := s.webdavUsers[key]; !ok {
		return ErrNotFound
	}
	delete(s.webdavUsers, key)
	return nil
}

// Close is a no-op for the in-memory store.
func (s *Memory) Close() error { return nil }

func eventMatches(ev events.Event, query string) bool {
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(ev.Type), q) || strings.Contains(strings.ToLower(ev.Severity), q) {
		return true
	}
	for k, v := range ev.Payload {
		if strings.Contains(strings.ToLower(k), q) || strings.Contains(strings.ToLower(toString(v)), q) {
			return true
		}
	}
	return false
}

func toString(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}
