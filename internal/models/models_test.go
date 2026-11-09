package models_test

import (
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/models"
)

func TestNewID(t *testing.T) {
	first := models.NewID("msg")
	second := models.NewID("msg")
	if !strings.HasPrefix(first, "msg_") || len(first) != len("msg_")+16 {
		t.Fatalf("first = %q", first)
	}
	if first == second {
		t.Fatalf("ids should be unique: %q", first)
	}
	if !strings.HasPrefix(models.NewID("pol"), "pol_") || !strings.HasPrefix(models.NewID("ev"), "ev_") {
		t.Fatal("prefix not applied")
	}
}

func TestMessageAddress(t *testing.T) {
	if got := (&models.Message{Mailbox: "inbox", Domain: "example.test"}).Address(); got != "inbox@example.test" {
		t.Fatalf("address = %q", got)
	}
	if got := (&models.Message{Recipient: "fallback@example.test"}).Address(); got != "fallback@example.test" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestPolicyTTL(t *testing.T) {
	if got := (models.Policy{TTLSeconds: 90}).TTL(); got != 90*time.Second {
		t.Fatalf("ttl = %v", got)
	}
	if got := (models.Policy{}).TTL(); got != 0 {
		t.Fatalf("zero ttl = %v", got)
	}
}

func TestSearchText(t *testing.T) {
	message := &models.Message{
		Sender: "Sender@Example.ORG", Recipient: "Inbox@Example.TEST",
		Mailbox: "Inbox", Domain: "Example.Test", Subject: "Hello World",
		RemoteIP: "203.0.113.9",
	}
	text := message.SearchText()
	for _, want := range []string{"sender@example.org", "inbox@example.test", "hello world", "203.0.113.9"} {
		if !strings.Contains(text, want) {
			t.Errorf("search text %q missing %q", text, want)
		}
	}
}

func TestNormalizeMailbox(t *testing.T) {
	mailbox, domain := models.NormalizeMailbox(" Inbox ", " Example.Test ")
	if mailbox != "Inbox" || domain != "example.test" {
		t.Fatalf("normalized = %q@%q", mailbox, domain)
	}
}

func TestSplitAddress(t *testing.T) {
	cases := []struct {
		address string
		mailbox string
		domain  string
	}{
		{"Inbox@Example.Test", "inbox", "example.test"},
		{"<user+tag@example.test>", "user+tag", "example.test"},
		{"  spaced@example.test  ", "spaced", "example.test"},
		{"no-at-sign", "no-at-sign", ""},
		{"", "", ""},
		{"user@", "user", ""},
	}
	for _, tc := range cases {
		mailbox, domain := models.SplitAddress(tc.address)
		if mailbox != tc.mailbox || domain != tc.domain {
			t.Errorf("SplitAddress(%q) = %q, %q; want %q, %q", tc.address, mailbox, domain, tc.mailbox, tc.domain)
		}
	}
}
