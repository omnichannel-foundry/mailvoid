package web_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
	searchidx "github.com/gnrfan/mailvoid/internal/search"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

func TestMCPTools(t *testing.T) {
	srv, store, bus := newTestServer(t)
	seedV1Messages(t, store)
	if err := store.SavePolicy(context.Background(), &models.Policy{
		Name: "keep all", Field: models.FieldAny, Pattern: ".*", Action: models.ActionKeep, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	bus.Emit(events.New(events.TypeMessageReceived, events.SeverityInfo, map[string]any{"test": true}))
	if err := store.SaveMessage(context.Background(), &models.Message{
		ID: "mcpfts", ReceivedAt: time.Now().UTC(), Mailbox: "inbox", Domain: "example.test",
		Recipient: "inbox@example.test", Sender: "fts@example.org", Subject: "Message",
		BodyText: "unique zebra body token", Action: models.ActionKeep,
	}); err != nil {
		t.Fatal(err)
	}
	index, _, err := searchidx.Open(filepath.Join(t.TempDir(), "mcp-search.bleve"))
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Ensure(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	srv.SetSearchIndex(index)
	t.Cleanup(func() { _ = index.Close() })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.App().Listener(ln) }()
	t.Cleanup(func() {
		_ = srv.Shutdown(context.Background())
		_ = ln.Close()
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "mailvoid-test", Version: "1.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint: "http://" + ln.Addr().String() + "/mcp",
		HTTPClient: &http.Client{
			Timeout:   20 * time.Second,
			Transport: bearerTransport{token: testAPIToken},
		},
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{
		"list_mailboxes", "check_mailbox", "wait_for_email", "get_message",
		"search_messages", "list_events", "list_policies", "get_stats",
		"delete_message",
	} {
		if !names[want] {
			t.Errorf("missing tool %s", want)
		}
	}

	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if result.IsError {
			t.Fatalf("%s returned tool error: %+v", name, result.Content)
		}
		if len(result.Content) == 0 && result.StructuredContent == nil {
			t.Fatalf("%s returned no content", name)
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	mailboxes := call("list_mailboxes", map[string]any{})
	if mailboxes["count"].(float64) != 2 {
		t.Fatalf("mailboxes = %+v", mailboxes)
	}

	checked := call("check_mailbox", map[string]any{"address": "inbox@example.test", "limit": 5})
	if checked["found"] != true {
		t.Fatalf("check = %+v", checked)
	}
	mailboxInfo, _ := checked["mailbox"].(map[string]any)
	if mailboxInfo["message_count"].(float64) != 3 {
		t.Fatalf("mailbox = %+v", mailboxInfo)
	}

	message := call("get_message", map[string]any{"id": "v1a", "include_raw": true})
	detail, _ := message["message"].(map[string]any)
	if detail["id"] != "v1a" || detail["raw"] != "raw-a" || detail["body_text"] != "body-a" {
		t.Fatalf("message = %+v", message)
	}

	searched := call("search_messages", map[string]any{"q": "invoice"})
	if searched["count"].(float64) != 1 || searched["backend"] != "bleve" {
		t.Fatalf("search = %+v", searched)
	}
	bodySearch := call("search_messages", map[string]any{"q": "zebra"})
	if bodySearch["count"].(float64) != 1 || bodySearch["backend"] != "bleve" {
		t.Fatalf("body search = %+v", bodySearch)
	}
	if bodySearch["highlights"] == nil {
		t.Fatalf("highlights missing: %+v", bodySearch)
	}

	waited := call("wait_for_email", map[string]any{
		"address":         "inbox@example.test",
		"since":           time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
		"timeout_seconds": 2,
	})
	if waited["found"] != true {
		t.Fatalf("wait = %+v", waited)
	}

	eventsOut := call("list_events", map[string]any{"limit": 10})
	if eventsOut["count"].(float64) < 1 {
		t.Fatalf("events = %+v", eventsOut)
	}

	policies := call("list_policies", map[string]any{})
	if policies["count"].(float64) != 1 {
		t.Fatalf("policies = %+v", policies)
	}

	stats := call("get_stats", map[string]any{})
	if stats["messages"].(float64) != 4 || stats["policies"].(float64) != 1 {
		t.Fatalf("stats = %+v", stats)
	}

	deleted := call("delete_message", map[string]any{"id": "v1a"})
	if deleted["deleted"] != true {
		t.Fatalf("delete = %+v", deleted)
	}
	if _, err := store.GetMessage(context.Background(), "v1a"); err == nil {
		t.Fatal("message should be deleted")
	}
}

func TestMCPRequiresAuth(t *testing.T) {
	srv, _, _ := newTestServer(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.App().Listener(ln) }()
	t.Cleanup(func() {
		_ = srv.Shutdown(context.Background())
		_ = ln.Close()
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "mailvoid-test", Version: "1.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:             "http://" + ln.Addr().String() + "/mcp",
		HTTPClient:           &http.Client{Timeout: 5 * time.Second},
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if session, err := client.Connect(ctx, transport, nil); err == nil {
		_ = session.Close()
		t.Fatal("expected authentication failure")
	}
}
