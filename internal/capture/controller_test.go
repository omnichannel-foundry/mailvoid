package capture_test

import (
	"context"
	"testing"

	"github.com/gnrfan/mailvoid/internal/capture"
	"github.com/gnrfan/mailvoid/internal/storage"
)

func TestLoadDefaultsWhenMissing(t *testing.T) {
	ctl, err := capture.Load(context.Background(), storage.NewMemory(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !ctl.Enabled() {
		t.Fatal("expected enabled by default")
	}
}

func TestSetPersists(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()
	ctl, err := capture.Load(ctx, store, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := ctl.Set(ctx, false); err != nil {
		t.Fatal(err)
	}
	if ctl.Enabled() {
		t.Fatal("expected disabled")
	}

	reloaded, err := capture.Load(ctx, store, true)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Enabled() {
		t.Fatal("persisted state should be disabled")
	}
}

func TestInvalidSettingFallsBack(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()
	if err := store.SetSetting(ctx, "capture.enabled", "not-a-bool"); err != nil {
		t.Fatal(err)
	}
	ctl, err := capture.Load(ctx, store, true)
	if err != nil {
		t.Fatal(err)
	}
	if !ctl.Enabled() {
		t.Fatal("invalid setting should fall back to the default")
	}
}
