package smtpserver_test

import (
	"context"
	"net/smtp"
	"strings"
	"testing"

	"github.com/gnrfan/mailvoid/internal/capture"
	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/policy"
	"github.com/gnrfan/mailvoid/internal/smtpserver"
	"github.com/gnrfan/mailvoid/internal/storage"
)

func TestCaptureDisabledRejectsMail(t *testing.T) {
	cfg := testConfig(0)
	store := storage.NewMemory()
	bus := events.NewBus(store)
	engine := policy.New(store)
	srv := smtpserver.New(cfg, store, engine, bus)
	srv.SetCaptureController(capture.New(false))
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	addr := srv.Addrs()[0].String()
	err := smtp.SendMail(addr, nil, "sender@example.org", []string{"inbox@example.test"}, []byte(sampleMessage))
	if err == nil || !strings.Contains(err.Error(), "capture disabled") {
		t.Fatalf("send error = %v", err)
	}

	msgs, err := store.ListMessages(context.Background(), models.MessageFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("messages = %d, want 0", len(msgs))
	}
	found := false
	for _, event := range bus.Recent(0) {
		if event.Type == events.TypeSMTPError && event.Payload["reason"] == "capture disabled" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("missing capture disabled smtp.error event")
	}
}

func TestCaptureToggleIsLive(t *testing.T) {
	cfg := testConfig(0)
	store := storage.NewMemory()
	bus := events.NewBus(store)
	engine := policy.New(store)
	controller := capture.New(false)
	srv := smtpserver.New(cfg, store, engine, bus)
	srv.SetCaptureController(controller)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	addr := srv.Addrs()[0].String()

	if err := smtp.SendMail(addr, nil, "a@example.org", []string{"x@example.test"}, []byte(sampleMessage)); err == nil {
		t.Fatal("expected rejection while capture is disabled")
	}
	if err := controller.Set(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := smtp.SendMail(addr, nil, "a@example.org", []string{"x@example.test"}, []byte(sampleMessage)); err != nil {
		t.Fatalf("send after enabling: %v", err)
	}
	msgs, _ := store.ListMessages(context.Background(), models.MessageFilter{})
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
}
