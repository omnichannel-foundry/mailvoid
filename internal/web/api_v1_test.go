package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
)

const testAPIToken = "agent-token"

func newEvent(typ, severity string) events.Event {
	return events.New(typ, severity, map[string]any{"test": true})
}

func v1Headers() map[string]string {
	return map[string]string{"Authorization": "Bearer " + testAPIToken}
}

func seedV1Messages(t *testing.T, store interface {
	SaveMessage(context.Context, *models.Message) error
}) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	for _, message := range []*models.Message{
		{ID: "v1a", ReceivedAt: now.Add(-3 * time.Minute), RemoteIP: "10.0.0.1", LocalPort: 1025, Domain: "example.test", Mailbox: "inbox", Recipient: "inbox@example.test", Sender: "one@example.org", Subject: "First invoice", Raw: "raw-a", BodyText: "body-a", Size: 5, Action: models.ActionKeep},
		{ID: "v1b", ReceivedAt: now.Add(-2 * time.Minute), RemoteIP: "10.0.0.2", LocalPort: 2525, Domain: "example.test", Mailbox: "inbox", Recipient: "inbox@example.test", Sender: "two@example.org", Subject: "Second notice", Raw: "raw-b", BodyText: "body-b", Size: 5, Action: models.ActionTTL},
		{ID: "v1c", ReceivedAt: now.Add(-1 * time.Minute), RemoteIP: "10.0.0.3", LocalPort: 1025, Domain: "other.test", Mailbox: "box", Recipient: "box@other.test", Sender: "three@example.org", Subject: "Third message", Raw: "raw-c", BodyText: "body-c", Size: 5, Action: models.ActionKeep},
	} {
		if err := store.SaveMessage(context.Background(), message); err != nil {
			t.Fatal(err)
		}
	}
}

func TestV1PublicDiscovery(t *testing.T) {
	srv, _, _ := newTestServer(t)
	for _, path := range []string{"/api/v1", "/api/v1/openapi.json", "/api/v1/openapi.yaml"} {
		resp := request(t, srv, http.MethodGet, path, nil, nil, nil)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", path, resp.StatusCode, body)
		}
	}
	resp := request(t, srv, http.MethodGet, "/api/v1/openapi.json", nil, nil, nil)
	body := readBody(t, resp)
	var spec struct {
		OpenAPI string         `json:"openapi"`
		Paths   map[string]any `json:"paths"`
	}
	if err := json.Unmarshal([]byte(body), &spec); err != nil {
		t.Fatal(err)
	}
	if spec.OpenAPI != "3.1.0" || spec.Paths["/mailboxes"] == nil || spec.Paths["/messages/{id}/eml"] == nil || spec.Paths["/search"] == nil || spec.Paths["/admin/api-keys"] == nil || spec.Paths["/admin/webdav-users"] == nil {
		t.Fatalf("spec = %+v", spec)
	}
}

func TestV1Authentication(t *testing.T) {
	srv, _, _ := newTestServer(t)

	resp := request(t, srv, http.MethodGet, "/api/v1/messages", nil, nil, nil)
	readBody(t, resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no auth status = %d", resp.StatusCode)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/messages", nil, nil, map[string]string{"Authorization": "Bearer wrong"})
	readBody(t, resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong token status = %d", resp.StatusCode)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/messages", nil, nil, map[string]string{"Authorization": "Basic abc"})
	readBody(t, resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong scheme status = %d", resp.StatusCode)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/messages", nil, nil, v1Headers())
	readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bearer status = %d", resp.StatusCode)
	}

	cookie := login(t, srv)
	resp = request(t, srv, http.MethodGet, "/api/v1/messages", nil, cookie, nil)
	readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("session status = %d", resp.StatusCode)
	}
}

func TestV1AuthenticationWithoutConfiguredToken(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIToken = ""
	srv, _, _ := newTestServerWithConfig(t, cfg)
	resp := request(t, srv, http.MethodGet, "/api/v1/messages", nil, nil, map[string]string{"Authorization": "Bearer anything"})
	readBody(t, resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestV1Mailboxes(t *testing.T) {
	srv, store, _ := newTestServer(t)
	seedV1Messages(t, store)

	resp := request(t, srv, http.MethodGet, "/api/v1/mailboxes", nil, nil, v1Headers())
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var list struct {
		Mailboxes []struct {
			Address      string `json:"address"`
			MessageCount int    `json:"message_count"`
		} `json:"mailboxes"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatal(err)
	}
	if list.Count != 2 || len(list.Mailboxes) != 2 || list.Mailboxes[0].Address != "box@other.test" {
		t.Fatalf("list = %+v", list)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/mailboxes?domain=example.test&limit=1&offset=1", nil, nil, v1Headers())
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"count":1`) {
		t.Fatalf("filtered status = %d, body = %s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/mailboxes/inbox@example.test", nil, nil, v1Headers())
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"message_count":2`) {
		t.Fatalf("summary status = %d, body = %s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/mailboxes/inbox@example.test/messages?limit=1", nil, nil, v1Headers())
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"count":2`) || !strings.Contains(body, "v1b") {
		t.Fatalf("messages status = %d, body = %s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/mailboxes/invalid", nil, nil, v1Headers())
	readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid address status = %d", resp.StatusCode)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/mailboxes?since=not-a-time", nil, nil, v1Headers())
	readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad since status = %d", resp.StatusCode)
	}
}

func TestV1WaitForMailbox(t *testing.T) {
	srv, store, _ := newTestServer(t)
	seedV1Messages(t, store)
	since := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)

	resp := request(t, srv, http.MethodGet, "/api/v1/mailboxes/inbox@example.test/wait?since="+since+"&timeout=1s", nil, nil, v1Headers())
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"found":true`) || !strings.Contains(body, "v1b") {
		t.Fatalf("wait status = %d, body = %s", resp.StatusCode, body)
	}

	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	resp = request(t, srv, http.MethodGet, "/api/v1/mailboxes/inbox@example.test/wait?since="+future+"&timeout=50ms", nil, nil, v1Headers())
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"found":false`) {
		t.Fatalf("timeout status = %d, body = %s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/mailboxes/inbox@example.test/wait?timeout=nope", nil, nil, v1Headers())
	readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad timeout status = %d", resp.StatusCode)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/mailboxes/invalid/wait", nil, nil, v1Headers())
	readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid address status = %d", resp.StatusCode)
	}
}

func TestV1MessagesFilters(t *testing.T) {
	srv, store, _ := newTestServer(t)
	seedV1Messages(t, store)

	resp := request(t, srv, http.MethodGet, "/api/v1/messages?q=invoice&limit=1", nil, nil, v1Headers())
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"count":1`) || !strings.Contains(body, "v1a") {
		t.Fatalf("q status = %d, body = %s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/messages?recipient=inbox@example.test&action=ttl&port=2525", nil, nil, v1Headers())
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "v1b") || strings.Contains(body, "v1a") {
		t.Fatalf("recipient status = %d, body = %s", resp.StatusCode, body)
	}

	invalid := []string{
		"?since=bad",
		"?until=bad",
		"?action=nope",
		"?port=0",
		"?port=70000",
		"?limit=0",
		"?offset=-1",
		"?page=0",
		"?recipient=bad",
	}
	for _, suffix := range invalid {
		resp = request(t, srv, http.MethodGet, "/api/v1/messages"+suffix, nil, nil, v1Headers())
		body = readBody(t, resp)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s status = %d, body = %s", suffix, resp.StatusCode, body)
		}
	}
}

func TestV1Events(t *testing.T) {
	srv, _, bus := newTestServer(t)
	bus.Emit(newEvent("message.received", "info"))
	bus.Emit(newEvent("message.dropped", "warn"))

	resp := request(t, srv, http.MethodGet, "/api/v1/events?type=message.dropped&limit=1", nil, nil, v1Headers())
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "message.dropped") || !strings.Contains(body, `"count":1`) {
		t.Fatalf("events status = %d, body = %s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/events?severity=warn", nil, nil, v1Headers())
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "message.dropped") {
		t.Fatalf("severity status = %d, body = %s", resp.StatusCode, body)
	}

	for _, suffix := range []string{"?severity=nope", "?since=bad", "?limit=0"} {
		resp = request(t, srv, http.MethodGet, "/api/v1/events"+suffix, nil, nil, v1Headers())
		readBody(t, resp)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s status = %d", suffix, resp.StatusCode)
		}
	}
}

func TestV1PoliciesAndStats(t *testing.T) {
	srv, store, _ := newTestServer(t)
	seedV1Messages(t, store)
	policy := &models.Policy{Name: "keep all", Field: models.FieldAny, Pattern: ".*", Action: models.ActionKeep, Enabled: true}
	if err := store.SavePolicy(context.Background(), policy); err != nil {
		t.Fatal(err)
	}

	resp := request(t, srv, http.MethodGet, "/api/v1/policies", nil, nil, v1Headers())
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "keep all") {
		t.Fatalf("policies status = %d, body = %s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/stats", nil, nil, v1Headers())
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"messages":3`) || !strings.Contains(body, `"policies":1`) {
		t.Fatalf("stats status = %d, body = %s", resp.StatusCode, body)
	}
}

func TestV1EMLDownload(t *testing.T) {
	srv, store, _ := newTestServer(t)
	seedV1Messages(t, store)

	resp := request(t, srv, http.MethodGet, "/api/v1/messages/v1a/eml", nil, nil, v1Headers())
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || body != "raw-a" {
		t.Fatalf("eml status = %d, body = %q", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "message/rfc822") {
		t.Fatalf("content type = %q", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, ".eml") {
		t.Fatalf("content disposition = %q", cd)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/messages/missing/eml", nil, nil, v1Headers())
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing eml status = %d", resp.StatusCode)
	}
}

func TestConsoleEMLDownload(t *testing.T) {
	srv, store, _ := newTestServer(t)
	seedV1Messages(t, store)
	cookie := login(t, srv)
	resp := request(t, srv, http.MethodGet, "/messages/v1b/eml", nil, cookie, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || body != "raw-b" {
		t.Fatalf("status = %d body = %q", resp.StatusCode, body)
	}
	resp = request(t, srv, http.MethodGet, "/messages/missing/eml", nil, cookie, nil)
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d", resp.StatusCode)
	}
}
