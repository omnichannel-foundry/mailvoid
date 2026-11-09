package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/models"
	searchidx "github.com/gnrfan/mailvoid/internal/search"
)

func TestV1FullTextSearch(t *testing.T) {
	srv, store, _ := newTestServer(t)
	now := time.Now().UTC()
	for _, message := range []*models.Message{
		{ID: "fts1", ReceivedAt: now.Add(-2 * time.Minute), RemoteIP: "10.0.0.1", LocalPort: 1025, Domain: "example.test", Mailbox: "inbox", Recipient: "inbox@example.test", Sender: "alice@example.org", Subject: "Welcome", BodyText: "The zebra is in the body", BodyHTML: "<p>zebra html</p>", Action: models.ActionKeep},
		{ID: "fts2", ReceivedAt: now.Add(-time.Minute), RemoteIP: "10.0.0.2", LocalPort: 2525, Domain: "other.test", Mailbox: "box", Recipient: "box@other.test", Sender: "bob@example.org", Subject: "Other", BodyText: "unrelated content", Action: models.ActionTTL},
	} {
		if err := store.SaveMessage(context.Background(), message); err != nil {
			t.Fatal(err)
		}
	}
	index, created, err := searchidx.Open(filepath.Join(t.TempDir(), "search.bleve"))
	if err != nil || !created {
		t.Fatalf("open = %v created=%v", err, created)
	}
	defer index.Close()
	if err := index.Ensure(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	srv.SetSearchIndex(index)
	cookie := login(t, srv)

	resp := request(t, srv, http.MethodGet, "/api/v1/search?q=zebra", nil, cookie, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search status = %d body = %s", resp.StatusCode, body)
	}
	var result struct {
		Messages   []map[string]any               `json:"messages"`
		Count      int                            `json:"count"`
		Backend    string                         `json:"backend"`
		Highlights map[string]map[string][]string `json:"highlights"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	if result.Backend != "bleve" || result.Count != 1 || len(result.Messages) != 1 || result.Messages[0]["id"] != "fts1" {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Highlights["fts1"]) == 0 {
		t.Fatalf("highlights = %+v", result.Highlights)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/search?q=zebra&mailbox=inbox&domain=example.test&port=1025&action=keep&sort=received_at", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !containsAll(body, `"count":1`, `"backend":"bleve"`) {
		t.Fatalf("filtered search = %d %s", resp.StatusCode, body)
	}
	resp = request(t, srv, http.MethodGet, "/api/v1/search?q=zebra&recipient=inbox@example.test&sender=alice@example.org", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !containsAll(body, `"count":1`) {
		t.Fatalf("recipient search = %d %s", resp.StatusCode, body)
	}
	resp = request(t, srv, http.MethodGet, "/api/v1/search?q=zebra&since="+now.Add(-3*time.Minute).Format(time.RFC3339), nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !containsAll(body, `"count":1`) {
		t.Fatalf("since search = %d %s", resp.StatusCode, body)
	}

	for _, suffix := range []string{"?action=nope", "?port=0", "?since=bad", "?sort=nope", "?limit=0", "?offset=-1"} {
		resp = request(t, srv, http.MethodGet, "/api/v1/search"+suffix, nil, cookie, nil)
		readBody(t, resp)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s status = %d", suffix, resp.StatusCode)
		}
	}
}

func TestV1SearchFallback(t *testing.T) {
	srv, store, _ := newTestServer(t)
	seedV1Messages(t, store)
	cookie := login(t, srv)
	resp := request(t, srv, http.MethodGet, "/api/v1/search?q=invoice", nil, cookie, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !containsAll(body, `"count":1`, `"backend":"fallback"`) {
		t.Fatalf("fallback = %d %s", resp.StatusCode, body)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
