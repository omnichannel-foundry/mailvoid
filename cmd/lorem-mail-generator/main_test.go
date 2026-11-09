package main

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"net/mail"
	"strconv"
	"strings"
	"testing"

	"github.com/gnrfan/mailvoid/internal/config"
	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/policy"
	"github.com/gnrfan/mailvoid/internal/smtpserver"
	"github.com/gnrfan/mailvoid/internal/storage"
)

func TestParseOptions(t *testing.T) {
	opts, err := parseOptions([]string{
		"-addr", "127.0.0.1:2025",
		"-from", "Sender <sender@example.test>",
		"-to", "one@example.test, Two <two@example.test>",
		"-count", "5",
		"-min-words", "10",
		"-max-words", "20",
		"-concurrency", "2",
		"-seed", "42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.addr != "127.0.0.1:2025" || opts.fromAddr != "sender@example.test" {
		t.Fatalf("opts = %+v", opts)
	}
	if len(opts.recipients) != 2 || opts.recipients[0] != "one@example.test" || opts.recipients[1] != "two@example.test" {
		t.Fatalf("recipients = %+v", opts.recipients)
	}
	if opts.count != 5 || opts.minWords != 10 || opts.maxWords != 20 || opts.concurrency != 2 || opts.seed != 42 {
		t.Fatalf("opts = %+v", opts)
	}
}

func TestParseOptionsErrors(t *testing.T) {
	cases := [][]string{
		{"-count", "0"},
		{"-min-words", "0"},
		{"-min-words", "10", "-max-words", "5"},
		{"-concurrency", "0"},
		{"-timeout", "0s"},
		{"-delay", "-1s"},
		{"-from", "not-an-address"},
		{"-to", "not-an-address"},
		{"-random-mailboxes", "-1"},
		{"-random-mailboxes", "5", "-domains", ""},
		{"-random-mailboxes", "5", "-domains", "bad domain"},
		{"unexpected"},
	}
	for _, args := range cases {
		if _, err := parseOptions(args); err == nil {
			t.Errorf("expected error for %v", args)
		}
	}
}

func TestParseOptionsRandomMailboxes(t *testing.T) {
	opts, err := parseOptions([]string{
		"-random-mailboxes", "12",
		"-domains", "Example.Test, test.local,example.test",
		"-mailbox-prefix", "box",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.randomMailboxes != 12 || opts.mailboxPrefix != "box" {
		t.Fatalf("opts = %+v", opts)
	}
	if len(opts.domains) != 2 || opts.domains[0] != "example.test" || opts.domains[1] != "test.local" {
		t.Fatalf("domains = %+v", opts.domains)
	}
}

func TestBuildMailboxPool(t *testing.T) {
	pool := buildMailboxPool(rand.New(rand.NewSource(7)), 25, []string{"example.test", "test.local"}, "box")
	if len(pool) != 25 {
		t.Fatalf("pool = %d", len(pool))
	}
	seen := make(map[string]bool)
	for _, address := range pool {
		if seen[address] {
			t.Fatalf("duplicate address %q", address)
		}
		seen[address] = true
		if !strings.HasPrefix(address, "box-") {
			t.Errorf("address %q missing prefix", address)
		}
		if !strings.HasSuffix(address, "@example.test") && !strings.HasSuffix(address, "@test.local") {
			t.Errorf("address %q has unexpected domain", address)
		}
	}
}

func TestLoremBodyWordCount(t *testing.T) {
	for _, words := range []int{1, 5, 10, 50, 120} {
		rng := rand.New(rand.NewSource(int64(words)))
		body := loremBody(rng, words, false)
		if got := len(strings.Fields(body)); got != words {
			t.Errorf("words=%d generated %d", words, got)
		}
	}
}

func TestBuildMessage(t *testing.T) {
	opts, err := parseOptions([]string{
		"-from", "lorem@example.test",
		"-to", "inbox@example.test",
		"-min-words", "7",
		"-max-words", "7",
		"-seed", "9",
	})
	if err != nil {
		t.Fatal(err)
	}
	message := buildMessage(opts, 3, rand.New(rand.NewSource(9)), opts.to)
	parsed, err := mail.ReadMessage(bytes.NewReader(message))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Header.Get("Subject") == "" || parsed.Header.Get("X-Mailvoid-Lorem-Index") != "3" {
		t.Fatalf("headers = %+v", parsed.Header)
	}
	if words, _ := strconv.Atoi(parsed.Header.Get("X-Mailvoid-Lorem-Words")); words != 7 {
		t.Fatalf("word header = %q", parsed.Header.Get("X-Mailvoid-Lorem-Words"))
	}
	body, err := io.ReadAll(parsed.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(strings.Fields(string(body))); got != 7 {
		t.Fatalf("body words = %d, want 7", got)
	}
}

func TestRunRandomMailboxes(t *testing.T) {
	cfg := config.Default()
	cfg.SMTPHost = "127.0.0.1"
	cfg.SMTPPorts = []int{0}
	cfg.Domains = []string{"*"}
	cfg.LogEnabled = false
	cfg.MaxMessageBytes = 1 << 20

	store := storage.NewMemory()
	bus := events.NewBus(store)
	engine := policy.New(store)
	srv := smtpserver.New(cfg, store, engine, bus)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	opts, err := parseOptions([]string{
		"-addr", srv.Addrs()[0].String(),
		"-count", "20",
		"-random-mailboxes", "6",
		"-domains", "example.test,test.local",
		"-concurrency", "4",
		"-min-words", "4",
		"-max-words", "12",
		"-seed", "11",
		"-quiet",
	})
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := run(opts, &out, &errOut); err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, errOut.String())
	}

	messages, err := store.ListMessages(context.Background(), models.MessageFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 20 {
		t.Fatalf("messages = %d, want 20", len(messages))
	}
	mailboxes := make(map[string]bool)
	for _, message := range messages {
		if message.Domain != "example.test" && message.Domain != "test.local" {
			t.Errorf("unexpected domain %q", message.Domain)
		}
		if !strings.HasPrefix(message.Mailbox, "lorem-") {
			t.Errorf("unexpected mailbox %q", message.Mailbox)
		}
		mailboxes[message.Recipient] = true
	}
	if len(mailboxes) < 2 {
		t.Fatalf("expected randomized mailboxes, got %v", mailboxes)
	}
}

func TestRunAgainstSMTP(t *testing.T) {
	cfg := config.Default()
	cfg.SMTPHost = "127.0.0.1"
	cfg.SMTPPorts = []int{0}
	cfg.Domains = []string{"*"}
	cfg.LogEnabled = false
	cfg.MaxMessageBytes = 1 << 20

	store := storage.NewMemory()
	bus := events.NewBus(store)
	engine := policy.New(store)
	srv := smtpserver.New(cfg, store, engine, bus)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	opts, err := parseOptions([]string{
		"-addr", srv.Addrs()[0].String(),
		"-count", "7",
		"-min-words", "5",
		"-max-words", "25",
		"-concurrency", "3",
		"-seed", "42",
		"-quiet",
	})
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := run(opts, &out, &errOut); err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, errOut.String())
	}

	messages, err := store.ListMessages(context.Background(), models.MessageFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 7 {
		t.Fatalf("messages = %d, want 7", len(messages))
	}
	seen := make(map[string]bool)
	for _, message := range messages {
		if seen[message.ID] {
			t.Fatalf("duplicate message id %q", message.ID)
		}
		seen[message.ID] = true
		words := len(strings.Fields(message.BodyText))
		if words < 5 || words > 25 {
			t.Errorf("message %s body words = %d, want 5..25", message.ID, words)
		}
		if message.Subject == "" {
			t.Errorf("message %s has empty subject", message.ID)
		}
	}
}
