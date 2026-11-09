// Package storage defines the repository interface used by MailVoid and
// provides in-memory and Genji-backed implementations.
package storage

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("storage: not found")

// Store is the persistence contract shared by the in-memory and Genji
// repositories. It is also an events.Sink so the event bus can persist events
// directly.
type Store interface {
	events.Sink

	SaveMessage(ctx context.Context, m *models.Message) error
	GetMessage(ctx context.Context, id string) (*models.Message, error)
	ListMessages(ctx context.Context, f models.MessageFilter) ([]*models.Message, error)
	CountMessages(ctx context.Context, f models.MessageFilter) (int, error)
	DeleteMessage(ctx context.Context, id string) error
	DeleteExpired(ctx context.Context, now time.Time) ([]*models.Message, error)

	SavePolicy(ctx context.Context, p *models.Policy) error
	GetPolicy(ctx context.Context, id string) (*models.Policy, error)
	ListPolicies(ctx context.Context) ([]*models.Policy, error)
	DeletePolicy(ctx context.Context, id string) error
	ReorderPolicies(ctx context.Context, ids []string) error

	SaveEvent(ctx context.Context, ev events.Event) error
	ListEvents(ctx context.Context, f models.EventFilter) ([]events.Event, error)
	CountEvents(ctx context.Context, f models.EventFilter) (int, error)

	GetUser(ctx context.Context, username string) (*models.User, error)
	ListUsers(ctx context.Context) ([]*models.User, error)
	SaveUser(ctx context.Context, u *models.User) error

	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error

	SaveAPIKey(ctx context.Context, key *models.APIKey) error
	GetAPIKey(ctx context.Context, id string) (*models.APIKey, error)
	GetAPIKeyByPrefix(ctx context.Context, prefix string) (*models.APIKey, error)
	ListAPIKeys(ctx context.Context) ([]*models.APIKey, error)
	DeleteAPIKey(ctx context.Context, id string) error

	SaveWebDAVUser(ctx context.Context, user *models.WebDAVUser) error
	GetWebDAVUser(ctx context.Context, username string) (*models.WebDAVUser, error)
	ListWebDAVUsers(ctx context.Context) ([]*models.WebDAVUser, error)
	DeleteWebDAVUser(ctx context.Context, username string) error

	Close() error
}

func normalizeMessage(m *models.Message) {
	m.Domain = strings.ToLower(strings.TrimSpace(m.Domain))
	m.Mailbox = strings.ToLower(strings.TrimSpace(m.Mailbox))
	m.Recipient = strings.ToLower(strings.TrimSpace(m.Recipient))
	m.Sender = strings.ToLower(strings.TrimSpace(m.Sender))
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func normalizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}
