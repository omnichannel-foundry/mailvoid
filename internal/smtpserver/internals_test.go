package smtpserver

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/config"
	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
)

func TestSplitAddress(t *testing.T) {
	cases := []struct {
		raw       string
		mailbox   string
		domain    string
		canonical string
		wantErr   bool
	}{
		{"Inbox@Example.Test", "inbox", "example.test", "inbox@example.test", false},
		{"<user+tag@example.test>", "user+tag", "example.test", "user+tag@example.test", false},
		{"", "", "", "", false},
		{"<>", "", "", "", false},
		{"bad", "", "", "bad", true},
		{"user@", "", "", "user@", true},
	}
	for _, tc := range cases {
		mailbox, domain, canonical, err := splitAddress(tc.raw)
		if (err != nil) != tc.wantErr {
			t.Errorf("%q err = %v, wantErr %v", tc.raw, err, tc.wantErr)
			continue
		}
		if mailbox != tc.mailbox || domain != tc.domain || canonical != tc.canonical {
			t.Errorf("%q = %q/%q/%q", tc.raw, mailbox, domain, canonical)
		}
	}
}

type fakeAddr string

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return string(a) }

func TestHostPortHelpers(t *testing.T) {
	if hostOnly(nil) != "" || portOnly(nil) != 0 {
		t.Fatal("nil addr")
	}
	if hostOnly(fakeAddr("not-a-hostport")) != "not-a-hostport" || portOnly(fakeAddr("not-a-hostport")) != 0 {
		t.Fatal("invalid addr")
	}
	if hostOnly(fakeAddr("127.0.0.1:1025")) != "127.0.0.1" || portOnly(fakeAddr("127.0.0.1:1025")) != 1025 {
		t.Fatal("valid addr")
	}
	var _ net.Addr = fakeAddr("")
}

func TestSessionMailRcptResetLogout(t *testing.T) {
	bus := events.NewBus()
	backend := &Backend{
		cfg: config.Config{Domains: []string{"allowed.test"}},
		bus: bus,
	}
	session := &session{backend: backend, remoteIP: "203.0.113.9", localPort: 1025}

	if err := session.Mail("bad", nil); err == nil {
		t.Fatal("expected invalid sender error")
	}
	if err := session.Mail("<Sender@Allowed.Test>", nil); err != nil {
		t.Fatal(err)
	}
	if session.from != "sender@allowed.test" {
		t.Fatalf("from = %q", session.from)
	}

	if err := session.Rcpt("bad", nil); err == nil {
		t.Fatal("expected invalid recipient error")
	}
	if err := session.Rcpt("x@denied.test", nil); err == nil || !strings.Contains(err.Error(), "relay access denied") {
		t.Fatalf("denied error = %v", err)
	}
	if err := session.Rcpt("Inbox@Allowed.Test", nil); err != nil {
		t.Fatal(err)
	}
	if len(session.recipients) != 1 || session.recipients[0] != "inbox@allowed.test" {
		t.Fatalf("recipients = %+v", session.recipients)
	}

	session.Reset()
	if session.from != "" || len(session.recipients) != 0 {
		t.Fatalf("reset = %+v", session)
	}
	if err := session.Logout(); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range bus.Recent(0) {
		if event.Type == events.TypeSMTPClosed {
			found = true
		}
	}
	if !found {
		t.Fatal("missing closed event")
	}
}

func TestDataWithoutRecipients(t *testing.T) {
	session := &session{backend: &Backend{bus: events.NewBus()}}
	if err := session.Data(strings.NewReader("x")); err == nil {
		t.Fatal("expected no-recipient error")
	}
}

func TestMessagePayload(t *testing.T) {
	expires := time.Now().UTC().Add(time.Hour)
	message := &models.Message{
		ID: "msg_1", RemoteIP: "1.2.3.4", LocalPort: 1025, Domain: "example.test",
		Mailbox: "inbox", Recipient: "inbox@example.test", Sender: "a@b.test",
		Subject: "hello", Size: 10, Action: models.ActionTTL, PolicyID: "pol_1",
		PolicyName: "ttl", ExpiresAt: &expires,
	}
	payload := messagePayload(message, "reason")
	if payload["message_id"] != "msg_1" || payload["expires_at"] == nil || payload["reason"] != "reason" {
		t.Fatalf("payload = %+v", payload)
	}
}
