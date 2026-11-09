// Package capture controls whether MailVoid stores and processes inbound mail.
package capture

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"

	"github.com/gnrfan/mailvoid/internal/storage"
)

const settingKey = "capture.enabled"

// Controller exposes a concurrency-safe capture switch and persists changes.
type Controller struct {
	store   storage.Store
	enabled atomic.Bool
}

// New creates an in-memory controller without persistence.
func New(enabled bool) *Controller {
	c := &Controller{}
	c.enabled.Store(enabled)
	return c
}

// Load restores the capture state from the database. A missing or invalid
// setting falls back to defaultEnabled.
func Load(ctx context.Context, store storage.Store, defaultEnabled bool) (*Controller, error) {
	c := &Controller{store: store}
	c.enabled.Store(defaultEnabled)
	value, err := store.GetSetting(ctx, settingKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return c, nil
		}
		return nil, err
	}
	if enabled, err := strconv.ParseBool(value); err == nil {
		c.enabled.Store(enabled)
	}
	return c, nil
}

// Enabled reports whether mail capture is currently on.
func (c *Controller) Enabled() bool {
	return c != nil && c.enabled.Load()
}

// Set persists and applies a new capture state.
func (c *Controller) Set(ctx context.Context, enabled bool) error {
	if c == nil {
		return nil
	}
	if c.store != nil {
		if err := c.store.SetSetting(ctx, settingKey, strconv.FormatBool(enabled)); err != nil {
			return err
		}
	}
	c.enabled.Store(enabled)
	return nil
}
