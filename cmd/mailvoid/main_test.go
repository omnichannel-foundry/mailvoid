package main

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/config"
	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/storage"
)

func testRunConfig(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	return config.Config{
		SMTPHost:        "127.0.0.1",
		SMTPPorts:       []int{0},
		HTTPAddr:        "127.0.0.1:0",
		Domains:         []string{"*"},
		DataDir:         filepath.Join(dir, "data"),
		DBPath:          filepath.Join(dir, "data", "mailvoid.db"),
		EventLogPath:    filepath.Join(dir, "data", "events.jsonl"),
		AdminUser:       "admin",
		AdminPassword:   "secret123",
		SweepInterval:   10 * time.Millisecond,
		SearchEnabled:   true,
		SearchIndexPath: filepath.Join(dir, "data", "search.bleve"),
		MaxMessageBytes: 1 << 20,
		ReadTimeout:     time.Second,
	}
}

func TestRunContextStartupAndShutdown(t *testing.T) {
	cfg := testRunConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runContext(ctx, cfg) }()
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runContext: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runContext did not stop")
	}

	data, err := os.ReadFile(cfg.EventLogPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, events.TypeSystemStartup) || !strings.Contains(text, events.TypeSystemShutdown) {
		t.Fatalf("event log = %s", text)
	}
	if _, err := os.Stat(cfg.DBPath); err != nil {
		t.Fatalf("database missing: %v", err)
	}
}

func TestRunContextErrors(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := testRunConfig(t)
	cfg.DataDir = blocker
	if err := runContext(context.Background(), cfg); err == nil {
		t.Fatal("expected data-dir error")
	}

	cfg = testRunConfig(t)
	dirPath := filepath.Join(dir, "event-dir")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg.EventLogPath = dirPath
	if err := runContext(context.Background(), cfg); err == nil {
		t.Fatal("expected event-log error")
	}

	cfg = testRunConfig(t)
	cfg.DBPath = blocker
	if err := runContext(context.Background(), cfg); err == nil {
		t.Fatal("expected database error")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	cfg = testRunConfig(t)
	cfg.SMTPPorts = []int{ln.Addr().(*net.TCPAddr).Port}
	if err := runContext(context.Background(), cfg); err == nil {
		t.Fatal("expected SMTP bind error")
	}
}

func TestSweepExpired(t *testing.T) {
	store := storage.NewMemory()
	bus := events.NewBus()
	now := time.Now().UTC()
	expired := &models.Message{ID: "old", ReceivedAt: now.Add(-time.Hour), ExpiresAt: timePtr(now.Add(-time.Minute)), Action: models.ActionTTL}
	future := &models.Message{ID: "new", ReceivedAt: now, ExpiresAt: timePtr(now.Add(time.Hour)), Action: models.ActionTTL}
	for _, message := range []*models.Message{expired, future} {
		if err := store.SaveMessage(context.Background(), message); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); sweepExpired(ctx, store, bus, 0) }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done

	if _, err := store.GetMessage(context.Background(), "old"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expired message still present: %v", err)
	}
	if _, err := store.GetMessage(context.Background(), "new"); err != nil {
		t.Fatalf("future message missing: %v", err)
	}
	found := false
	for _, event := range bus.Recent(0) {
		if event.Type == events.TypeMessageDeleted {
			found = true
		}
	}
	if !found {
		t.Fatal("expected deletion event")
	}
}

func TestSweepExpiredTicker(t *testing.T) {
	store := storage.NewMemory()
	bus := events.NewBus()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	sweepExpired(ctx, store, bus, 5*time.Millisecond)
}

type failingDeleteStore struct {
	storage.Store
	err error
}

func (f failingDeleteStore) DeleteExpired(context.Context, time.Time) ([]*models.Message, error) {
	return nil, f.err
}

func TestSweepExpiredError(t *testing.T) {
	store := failingDeleteStore{Store: storage.NewMemory(), err: errors.New("sweep failed")}
	bus := events.NewBus()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	sweepExpired(ctx, store, bus, 5*time.Millisecond)
	found := false
	for _, event := range bus.Recent(0) {
		if event.Type == events.TypeMessageError && event.Payload["reason"] == "ttl sweep" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected sweep error event")
	}
}

func timePtr(value time.Time) *time.Time { return &value }

func TestMainVersionFlag(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"mailvoid", "--version"}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = write
	main()
	_ = write.Close()
	os.Stdout = oldStdout
	buf := make([]byte, 256)
	n, _ := read.Read(buf)
	_ = read.Close()
	if !strings.Contains(string(buf[:n]), "mailvoid") {
		t.Fatalf("output = %q", buf[:n])
	}
}

func TestRunWrapperError(t *testing.T) {
	cfg := testRunConfig(t)
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.DataDir = blocker
	if err := run(cfg); err == nil {
		t.Fatal("expected run error")
	}
}
