package web

import (
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/mailbox"
	"github.com/gnrfan/mailvoid/internal/models"
)

func TestMCPHelpers(t *testing.T) {
	if _, err := parseMCPTime("bad", "since"); err == nil {
		t.Fatal("expected time error")
	}
	if got, err := parseMCPTime("", "since"); err != nil || !got.IsZero() {
		t.Fatalf("empty time = %v %v", got, err)
	}
	if limit, offset := mcpPage(0, -1); limit != 50 || offset != 0 {
		t.Fatalf("page defaults = %d/%d", limit, offset)
	}
	if limit, _ := mcpPage(999, 0); limit != 200 {
		t.Fatalf("page max = %d", limit)
	}

	if _, err := mcpMessageFilter(searchMessagesInput{Since: "bad"}); err == nil {
		t.Fatal("expected since error")
	}
	if _, err := mcpMessageFilter(searchMessagesInput{Until: "bad"}); err == nil {
		t.Fatal("expected until error")
	}
	filter, err := mcpMessageFilter(searchMessagesInput{Recipient: "Inbox@Example.Test", Limit: 999, Offset: -2})
	if err != nil {
		t.Fatal(err)
	}
	if filter.Mailbox != "inbox" || filter.Domain != "example.test" || filter.Limit != 200 || filter.Offset != 0 {
		t.Fatalf("filter = %+v", filter)
	}
	if _, err := mcpMessageFilter(searchMessagesInput{Recipient: "bad"}); err == nil {
		t.Fatal("expected recipient error")
	}

	expires := time.Now().UTC().Add(time.Hour)
	message := &models.Message{
		ID: "m", ReceivedAt: time.Now().UTC(), Mailbox: "inbox", Domain: "example.test",
		ExpiresAt: &expires, Raw: "raw", Headers: map[string][]string{"A": {"b"}},
	}
	detail := mcpDetailFrom(message, true)
	if detail.Raw != "raw" || detail.ExpiresAt == "" || detail.Headers["A"][0] != "b" {
		t.Fatalf("detail = %+v", detail)
	}
	if mcpDetailFrom(message, false).Raw != "" {
		t.Fatal("raw should be omitted")
	}
	policy := &models.Policy{ID: "p", Name: "n", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if mcpPolicyFrom(policy).ID != "p" {
		t.Fatal("policy conversion")
	}
	event := events.New(events.TypeMessageReceived, events.SeverityInfo, map[string]any{"x": 1})
	if mcpEventFrom(event).Type != events.TypeMessageReceived {
		t.Fatal("event conversion")
	}
	latest := time.Now().UTC()
	if mcpMailboxFrom(mailbox.Summary{Address: "a@b.test", LatestReceivedAt: &latest}).LatestReceivedAt == "" {
		t.Fatal("mailbox timestamp")
	}
	if mcpMailboxFrom(mailbox.Summary{Address: "a@b.test"}).LatestReceivedAt != "" {
		t.Fatal("empty timestamp")
	}
}
