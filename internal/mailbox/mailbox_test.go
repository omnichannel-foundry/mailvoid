package mailbox_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/mailbox"
	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/storage"
)

func seed(t *testing.T, store storage.Store) (base time.Time, messages map[string]*models.Message) {
	t.Helper()
	base = time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	defs := []*models.Message{
		{ID: "m1", Mailbox: "inbox", Domain: "a.test", Recipient: "inbox@a.test", Sender: "one@x.test", Subject: "first", RemoteIP: "10.0.0.1", LocalPort: 1025, ReceivedAt: base.Add(1 * time.Minute), Action: models.ActionKeep},
		{ID: "m2", Mailbox: "other", Domain: "a.test", Recipient: "other@a.test", Sender: "two@x.test", Subject: "second", RemoteIP: "10.0.0.2", LocalPort: 2525, ReceivedAt: base.Add(2 * time.Minute), Action: models.ActionTTL},
		{ID: "m3", Mailbox: "inbox", Domain: "a.test", Recipient: "inbox@a.test", Sender: "three@x.test", Subject: "third", RemoteIP: "10.0.0.3", LocalPort: 1025, ReceivedAt: base.Add(3 * time.Minute), Action: models.ActionKeep},
		{ID: "m4", Mailbox: "box", Domain: "b.test", Recipient: "box@b.test", Sender: "four@x.test", Subject: "fourth", RemoteIP: "10.0.0.4", LocalPort: 1025, ReceivedAt: base.Add(4 * time.Minute), Action: models.ActionKeep},
	}
	messages = map[string]*models.Message{}
	for _, message := range defs {
		if err := store.SaveMessage(context.Background(), message); err != nil {
			t.Fatal(err)
		}
		messages[message.ID] = message
	}
	return base, messages
}

func TestParseAddress(t *testing.T) {
	valid := []string{"inbox@example.test", "Inbox@Example.Test", "<user+tag@example.test>"}
	for _, address := range valid {
		mailboxName, domain, err := mailbox.ParseAddress(address)
		if err != nil {
			t.Fatalf("%q: %v", address, err)
		}
		if mailboxName == "" || domain == "" {
			t.Fatalf("%q: got %q@%q", address, mailboxName, domain)
		}
	}
	for _, address := range []string{"", "no-at-sign", "user name@example.test", "@example.test", "user@"} {
		if _, _, err := mailbox.ParseAddress(address); !errors.Is(err, mailbox.ErrInvalidAddress) {
			t.Fatalf("%q: err = %v", address, err)
		}
	}
	if mailboxName, domain, _ := mailbox.ParseAddress("Inbox@Example.Test"); mailboxName != "inbox" || domain != "example.test" {
		t.Fatalf("normalization = %q@%q", mailboxName, domain)
	}
}

func TestAllAndList(t *testing.T) {
	store := storage.NewMemory()
	base, _ := seed(t, store)
	service := mailbox.New(store)
	ctx := context.Background()

	all, err := service.All(ctx, "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("all = %+v", all)
	}
	if all[0].Address != "box@b.test" || all[1].Address != "inbox@a.test" || all[2].Address != "other@a.test" {
		t.Fatalf("order = %+v", all)
	}
	if all[1].MessageCount != 2 || all[1].LatestMessageID != "m3" || all[1].LatestSubject != "third" {
		t.Fatalf("inbox summary = %+v", all[1])
	}

	page, total, err := service.List(ctx, mailbox.ListOptions{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(page) != 2 || page[0].Address != "inbox@a.test" {
		t.Fatalf("page = %+v total=%d", page, total)
	}
	empty, total, err := service.List(ctx, mailbox.ListOptions{Limit: 2, Offset: 99})
	if err != nil || total != 3 || len(empty) != 0 {
		t.Fatalf("empty page = %+v total=%d err=%v", empty, total, err)
	}

	onlyA, err := service.All(ctx, "a.test", time.Time{})
	if err != nil || len(onlyA) != 2 {
		t.Fatalf("domain filter = %+v err=%v", onlyA, err)
	}
	recent, err := service.All(ctx, "", base.Add(3*time.Minute+30*time.Second))
	if err != nil || len(recent) != 1 || recent[0].Address != "box@b.test" {
		t.Fatalf("since filter = %+v err=%v", recent, err)
	}
}

func TestSummary(t *testing.T) {
	store := storage.NewMemory()
	seed(t, store)
	service := mailbox.New(store)
	ctx := context.Background()

	summary, err := service.Summary(ctx, "inbox@a.test", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if summary.MessageCount != 2 || summary.LatestMessageID != "m3" || summary.LatestReceivedAt == nil {
		t.Fatalf("summary = %+v", summary)
	}
	if !strings.Contains(summary.String(), "inbox@a.test") {
		t.Fatalf("string = %q", summary.String())
	}

	empty, err := service.Summary(ctx, "missing@a.test", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if empty.MessageCount != 0 || empty.LatestReceivedAt != nil {
		t.Fatalf("empty = %+v", empty)
	}

	if _, err := service.Summary(ctx, "bad", time.Time{}); !errors.Is(err, mailbox.ErrInvalidAddress) {
		t.Fatalf("err = %v", err)
	}
}

func TestMessages(t *testing.T) {
	store := storage.NewMemory()
	base, _ := seed(t, store)
	service := mailbox.New(store)
	ctx := context.Background()

	messages, total, err := service.Messages(ctx, "inbox@a.test", mailbox.MessageOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(messages) != 2 || messages[0].ID != "m3" {
		t.Fatalf("messages = %+v total=%d", messages, total)
	}

	page, total, err := service.Messages(ctx, "inbox@a.test", mailbox.MessageOptions{Limit: 1, Offset: 1})
	if err != nil || total != 2 || len(page) != 1 || page[0].ID != "m1" {
		t.Fatalf("page = %+v total=%d err=%v", page, total, err)
	}

	filtered, total, err := service.Messages(ctx, "inbox@a.test", mailbox.MessageOptions{
		Query: "third", Since: base.Add(2 * time.Minute), Until: base.Add(4 * time.Minute), Port: 1025,
	})
	if err != nil || total != 1 || len(filtered) != 1 || filtered[0].ID != "m3" {
		t.Fatalf("filtered = %+v total=%d err=%v", filtered, total, err)
	}

	if _, _, err := service.Messages(ctx, "bad", mailbox.MessageOptions{}); !errors.Is(err, mailbox.ErrInvalidAddress) {
		t.Fatalf("err = %v", err)
	}
}

func TestAllMessages(t *testing.T) {
	store := storage.NewMemory()
	seed(t, store)
	service := mailbox.New(store)
	messages, err := service.AllMessages(context.Background(), "inbox@a.test", mailbox.MessageOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %+v", messages)
	}
	if _, err := service.AllMessages(context.Background(), "bad", mailbox.MessageOptions{}); !errors.Is(err, mailbox.ErrInvalidAddress) {
		t.Fatalf("err = %v", err)
	}
}

func TestWait(t *testing.T) {
	store := storage.NewMemory()
	base, _ := seed(t, store)
	service := mailbox.New(store)
	ctx := context.Background()

	message, err := service.Wait(ctx, "inbox@a.test", base.Add(2*time.Minute+30*time.Second), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if message.ID != "m3" {
		t.Fatalf("message = %+v", message)
	}

	if _, err := service.Wait(ctx, "inbox@a.test", time.Now().Add(time.Hour), 50*time.Millisecond); !errors.Is(err, mailbox.ErrTimeout) {
		t.Fatalf("timeout err = %v", err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := service.Wait(cancelled, "inbox@a.test", time.Now().Add(time.Hour), time.Second); !errors.Is(err, mailbox.ErrTimeout) {
		t.Fatalf("cancel err = %v", err)
	}

	if _, err := service.Wait(ctx, "bad", time.Time{}, time.Millisecond); !errors.Is(err, mailbox.ErrInvalidAddress) {
		t.Fatalf("address err = %v", err)
	}

	// A zero since starts at "now"; a pre-existing message must not be returned.
	if _, err := service.Wait(ctx, "inbox@a.test", time.Time{}, 20*time.Millisecond); !errors.Is(err, mailbox.ErrTimeout) {
		t.Fatalf("zero-since err = %v", err)
	}
}
