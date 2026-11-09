package web_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/auth"
	"github.com/gnrfan/mailvoid/internal/config"
	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/policy"
	"github.com/gnrfan/mailvoid/internal/storage"
	"github.com/gnrfan/mailvoid/internal/web"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.DBPath = t.TempDir() + "/unused.db"
	cfg.EventLogPath = t.TempDir() + "/unused.jsonl"
	cfg.AdminUser = "admin"
	cfg.AdminPassword = "secret123"
	cfg.APIToken = "agent-token"
	cfg.WebDAVUser = "dav"
	cfg.WebDAVPassword = "dav-secret"
	cfg.SMTPPorts = []int{1025, 2525}
	return cfg
}

func newTestServer(t *testing.T) (*web.Server, storage.Store, *events.Bus) {
	t.Helper()
	return newTestServerWithConfig(t, testConfig(t))
}

func newTestServerWithConfig(t *testing.T, cfg config.Config) (*web.Server, storage.Store, *events.Bus) {
	t.Helper()
	store := storage.NewMemory()
	bus := events.NewBus(store)
	engine := policy.New(store)
	if _, err := auth.New(store).EnsureBootstrap(context.Background(), cfg.AdminUser, cfg.AdminPassword, false); err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(cfg, store, bus, engine, "test")
	if err != nil {
		t.Fatal(err)
	}
	return srv, store, bus
}

func request(t *testing.T, srv *web.Server, method, target string, body io.Reader, cookie *http.Cookie, headers map[string]string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := srv.App().Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return string(b)
}

func login(t *testing.T, srv *web.Server) *http.Cookie {
	t.Helper()
	form := url.Values{"username": {"admin"}, "password": {"secret123"}, "next": {"/"}}
	resp := request(t, srv, http.MethodPost, "/login", strings.NewReader(form.Encode()), nil,
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login status = %d, body = %s", resp.StatusCode, body)
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "mailvoid_session" {
			return cookie
		}
	}
	t.Fatal("session cookie not set")
	return nil
}

func TestAuthenticationGate(t *testing.T) {
	srv, _, _ := newTestServer(t)

	resp := request(t, srv, http.MethodGet, "/", nil, nil, nil)
	readBody(t, resp)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("dashboard status = %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "/login") {
		t.Fatalf("location = %q", loc)
	}

	resp = request(t, srv, http.MethodGet, "/api/messages", nil, nil, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("api status = %d, body = %s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodGet, "/login", nil, nil, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Sign in") {
		t.Fatalf("login page status = %d", resp.StatusCode)
	}
}

func TestLoginFailureEmitsEvent(t *testing.T) {
	srv, _, bus := newTestServer(t)
	form := url.Values{"username": {"admin"}, "password": {"wrong"}}
	resp := request(t, srv, http.MethodPost, "/login", strings.NewReader(form.Encode()), nil,
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(body, "Invalid username or password") {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	found := false
	for _, ev := range bus.Recent(0) {
		if ev.Type == events.TypeAuthLoginFailed {
			found = true
		}
	}
	if !found {
		t.Fatal("expected auth.login.failed event")
	}
}

func TestLoginAndDashboard(t *testing.T) {
	srv, store, _ := newTestServer(t)
	if err := store.SaveMessage(context.Background(), &models.Message{
		ID: "m1", ReceivedAt: time.Now().UTC(), RemoteIP: "127.0.0.1", LocalPort: 1025,
		Domain: "example.test", Mailbox: "inbox", Recipient: "inbox@example.test",
		Sender: "sender@example.org", Subject: "Dashboard message", Action: models.ActionKeep,
	}); err != nil {
		t.Fatal(err)
	}
	cookie := login(t, srv)

	resp := request(t, srv, http.MethodGet, "/", nil, cookie, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Dashboard") || !strings.Contains(body, "Dashboard message") {
		t.Fatalf("dashboard body missing content")
	}
}

func TestMessagePagesAndAPI(t *testing.T) {
	srv, store, _ := newTestServer(t)
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	msg := &models.Message{
		ID: "msg_api", ReceivedAt: time.Now().UTC(), RemoteIP: "198.51.100.4", LocalPort: 2525,
		Domain: "example.test", Mailbox: "box", Recipient: "box@example.test",
		Sender: "a@b.test", Subject: "API subject", Headers: map[string][]string{"X-Test": {"yes"}},
		Raw: "raw", BodyText: "body text", BodyHTML: "<b>body</b>", Size: 3, Action: models.ActionTTL,
		ExpiresAt: &expires,
	}
	if err := store.SaveMessage(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	cookie := login(t, srv)

	resp := request(t, srv, http.MethodGet, "/api/messages?q=api", nil, cookie, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("api status = %d, body = %s", resp.StatusCode, body)
	}
	var list struct {
		Messages []struct {
			ID      string `json:"id"`
			Subject string `json:"subject"`
		} `json:"messages"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatal(err)
	}
	if list.Count != 1 || list.Messages[0].ID != "msg_api" {
		t.Fatalf("list = %+v", list)
	}

	resp = request(t, srv, http.MethodGet, "/api/messages/msg_api", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "raw") || !strings.Contains(body, "body text") {
		t.Fatalf("detail status = %d, body = %s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodGet, "/messages/msg_api", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "API subject") || !strings.Contains(body, "198.51.100.4") {
		t.Fatalf("detail page status = %d", resp.StatusCode)
	}
	if !strings.Contains(body, `data-expires="`+expires.Format(time.RFC3339)) {
		t.Fatalf("detail page missing RFC3339 expiry")
	}

	resp = request(t, srv, http.MethodDelete, "/api/messages/msg_api", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "true") {
		t.Fatalf("delete status = %d, body = %s", resp.StatusCode, body)
	}
	if _, err := store.GetMessage(context.Background(), "msg_api"); err == nil {
		t.Fatal("message should be deleted")
	}
}

func TestMessageListHTMXFragment(t *testing.T) {
	srv, _, _ := newTestServer(t)
	cookie := login(t, srv)
	resp := request(t, srv, http.MethodGet, "/messages", nil, cookie, map[string]string{"HX-Request": "true"})
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if strings.Contains(body, "<!DOCTYPE html>") || !strings.Contains(body, `id="message-list"`) {
		t.Fatalf("expected fragment, got: %.200s", body)
	}
}

func TestPolicyAPIAndPages(t *testing.T) {
	srv, _, bus := newTestServer(t)
	cookie := login(t, srv)
	jsonHeaders := map[string]string{"Content-Type": "application/json"}

	resp := request(t, srv, http.MethodPost, "/api/policies",
		strings.NewReader(`{"name":"drop spam","field":"sender","pattern":"@spam\\.test$","action":"drop","enabled":true}`),
		cookie, jsonHeaders)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", resp.StatusCode, body)
	}
	var first models.Policy
	if err := json.Unmarshal([]byte(body), &first); err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.Action != models.ActionDrop {
		t.Fatalf("policy = %+v", first)
	}

	resp = request(t, srv, http.MethodPost, "/api/policies",
		strings.NewReader(`{"name":"ttl","field":"subject","pattern":"hello","action":"ttl","ttl_seconds":60,"enabled":true}`),
		cookie, jsonHeaders)
	body = readBody(t, resp)
	var second models.Policy
	_ = json.Unmarshal([]byte(body), &second)
	if resp.StatusCode != http.StatusCreated || second.ID == "" {
		t.Fatalf("second create status = %d, body = %s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodGet, "/api/policies", nil, cookie, nil)
	body = readBody(t, resp)
	if !strings.Contains(body, `"count":2`) {
		t.Fatalf("list body = %s", body)
	}

	resp = request(t, srv, http.MethodPost, "/api/policies/reorder",
		strings.NewReader(`{"ids":["`+second.ID+`","`+first.ID+`"]}`), cookie, jsonHeaders)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reorder status = %d, body = %s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodPost, "/api/policies/test",
		strings.NewReader(`{"field":"subject","pattern":"^hello$","value":"hello"}`), cookie, jsonHeaders)
	body = readBody(t, resp)
	if !strings.Contains(body, `"matched":true`) {
		t.Fatalf("test body = %s", body)
	}

	resp = request(t, srv, http.MethodPut, "/api/policies/"+first.ID,
		strings.NewReader(`{"name":"drop spam v2","field":"sender","pattern":"@spam\\.test$","action":"drop","enabled":false}`),
		cookie, jsonHeaders)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "drop spam v2") || !strings.Contains(body, `"enabled":false`) {
		t.Fatalf("update status = %d, body = %s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodGet, "/policies", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "drop spam v2") {
		t.Fatalf("policies page status = %d", resp.StatusCode)
	}

	resp = request(t, srv, http.MethodGet, "/policies/"+first.ID+"/edit", nil, cookie, map[string]string{"HX-Request": "true"})
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Save policy") {
		t.Fatalf("edit fragment status = %d, body = %.200s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodDelete, "/api/policies/"+first.ID, nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", resp.StatusCode, body)
	}
	types := map[string]bool{}
	for _, ev := range bus.Recent(0) {
		types[ev.Type] = true
	}
	for _, want := range []string{events.TypePolicyCreated, events.TypePolicyUpdated, events.TypePolicyReordered, events.TypePolicyDeleted} {
		if !types[want] {
			t.Errorf("missing event %s", want)
		}
	}
}

func TestEventsAPIAndStats(t *testing.T) {
	srv, _, _ := newTestServer(t)
	cookie := login(t, srv)

	resp := request(t, srv, http.MethodGet, "/api/events?type=auth.login.success", nil, cookie, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "auth.login.success") {
		t.Fatalf("events status = %d, body = %s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodGet, "/api/stats", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"messages":0`) {
		t.Fatalf("stats status = %d, body = %s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodGet, "/events", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Event stream") {
		t.Fatalf("events page status = %d", resp.StatusCode)
	}
}

func TestSettingsAndPasswordChange(t *testing.T) {
	srv, _, _ := newTestServer(t)
	cookie := login(t, srv)

	resp := request(t, srv, http.MethodGet, "/settings", nil, cookie, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Administrator password") {
		t.Fatalf("settings status = %d", resp.StatusCode)
	}

	form := url.Values{"current": {"secret123"}, "password": {"newsecret123"}, "confirm": {"newsecret123"}}
	resp = request(t, srv, http.MethodPost, "/settings/password", strings.NewReader(form.Encode()), cookie,
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("password change status = %d, body = %s", resp.StatusCode, body)
	}

	loginForm := url.Values{"username": {"admin"}, "password": {"newsecret123"}}
	resp = request(t, srv, http.MethodPost, "/login", strings.NewReader(loginForm.Encode()), nil,
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	readBody(t, resp)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login with new password status = %d", resp.StatusCode)
	}
}

func TestNotFoundRendersErrorPage(t *testing.T) {
	srv, _, _ := newTestServer(t)
	cookie := login(t, srv)
	resp := request(t, srv, http.MethodGet, "/messages/missing", nil, cookie, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !strings.Contains(body, "message not found") {
		t.Fatalf("body = %.300s", body)
	}

	resp = request(t, srv, http.MethodGet, "/api/messages/missing", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(body, "message not found") {
		t.Fatalf("api status = %d, body = %s", resp.StatusCode, body)
	}
}

func TestLogout(t *testing.T) {
	srv, _, bus := newTestServer(t)
	cookie := login(t, srv)
	resp := request(t, srv, http.MethodPost, "/logout", nil, cookie, nil)
	readBody(t, resp)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("logout status = %d", resp.StatusCode)
	}
	found := false
	for _, ev := range bus.Recent(0) {
		if ev.Type == events.TypeAuthLogout {
			found = true
		}
	}
	if !found {
		t.Fatal("expected auth.logout event")
	}
}
