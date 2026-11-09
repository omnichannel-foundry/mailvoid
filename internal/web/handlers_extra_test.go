package web_test

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/web"
)

func seedMessage(t *testing.T, store interface {
	SaveMessage(context.Context, *models.Message) error
}, id, mailbox, domain string) *models.Message {
	t.Helper()
	message := &models.Message{
		ID: id, ReceivedAt: time.Now().UTC(), RemoteIP: "127.0.0.1", LocalPort: 1025,
		Mailbox: mailbox, Domain: domain, Recipient: mailbox + "@" + domain,
		Sender: "sender@example.org", Subject: "Subject " + id, Raw: "raw-" + id,
		BodyText: "body", Size: 10, Action: models.ActionKeep,
	}
	if err := store.SaveMessage(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	return message
}

func seedPolicy(t *testing.T, store interface {
	SavePolicy(context.Context, *models.Policy) error
}, policy *models.Policy) *models.Policy {
	t.Helper()
	if err := store.SavePolicy(context.Background(), policy); err != nil {
		t.Fatal(err)
	}
	return policy
}

func postFormRequest(t *testing.T, srv *web.Server, path string, values url.Values, cookie *http.Cookie) *http.Response {
	t.Helper()
	return request(t, srv, http.MethodPost, path, strings.NewReader(values.Encode()), cookie,
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
}

func TestMessageDeleteUI(t *testing.T) {
	srv, store, bus := newTestServer(t)
	seedMessage(t, store, "ui1", "inbox", "example.test")
	cookie := login(t, srv)

	resp := postFormRequest(t, srv, "/messages/ui1/delete", url.Values{}, cookie)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if _, err := store.GetMessage(context.Background(), "ui1"); err == nil {
		t.Fatal("message should be deleted")
	}
	found := false
	for _, event := range bus.Recent(0) {
		if event.Type == events.TypeMessageDeleted && event.Payload["reason"] == "manual deletion" && event.Payload["username"] == "admin" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected manual deletion event")
	}

	resp = postFormRequest(t, srv, "/messages/missing/delete", url.Values{}, cookie)
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d", resp.StatusCode)
	}
}

func TestPolicyUICreateAndValidation(t *testing.T) {
	srv, store, bus := newTestServer(t)
	cookie := login(t, srv)

	form := url.Values{
		"name": {"drop spam"}, "field": {"sender"}, "pattern": {`@spam\.test$`},
		"action": {"drop"}, "enabled": {"1"},
	}
	resp := postFormRequest(t, srv, "/policies", form, cookie)
	readBody(t, resp)
	if resp.StatusCode != http.StatusFound || !strings.Contains(resp.Header.Get("Location"), "flash=") {
		t.Fatalf("create status = %d location = %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	policies, _ := store.ListPolicies(context.Background())
	if len(policies) != 1 || policies[0].Name != "drop spam" {
		t.Fatalf("policies = %+v", policies)
	}
	found := false
	for _, event := range bus.Recent(0) {
		if event.Type == events.TypePolicyCreated {
			found = true
		}
	}
	if !found {
		t.Fatal("expected policy.created event")
	}

	resp = postFormRequest(t, srv, "/policies", url.Values{"field": {"sender"}, "pattern": {"x"}, "action": {"keep"}}, cookie)
	readBody(t, resp)
	if resp.StatusCode != http.StatusFound || !strings.Contains(resp.Header.Get("Location"), "error=") {
		t.Fatalf("invalid create = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestPolicyUIUpdateToggleMoveDelete(t *testing.T) {
	srv, store, bus := newTestServer(t)
	cookie := login(t, srv)
	first := seedPolicy(t, store, &models.Policy{Name: "first", Field: models.FieldSender, Pattern: "a", Action: models.ActionKeep, Enabled: true})
	second := seedPolicy(t, store, &models.Policy{Name: "second", Field: models.FieldSubject, Pattern: "b", Action: models.ActionKeep, Enabled: true})
	third := seedPolicy(t, store, &models.Policy{Name: "third", Field: models.FieldDomain, Pattern: "c", Action: models.ActionKeep, Enabled: true})

	update := url.Values{
		"name": {"first updated"}, "field": {"sender"}, "pattern": {`@updated\.test$`},
		"action": {"ttl"}, "ttl_seconds": {"120"}, "enabled": {"1"},
	}
	resp := postFormRequest(t, srv, "/policies/"+first.ID, update, cookie)
	readBody(t, resp)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("update status = %d", resp.StatusCode)
	}
	updated, _ := store.GetPolicy(context.Background(), first.ID)
	if updated.Name != "first updated" || updated.Action != models.ActionTTL || updated.TTLSeconds != 120 || updated.Priority != first.Priority {
		t.Fatalf("updated = %+v", updated)
	}

	resp = postFormRequest(t, srv, "/policies/"+first.ID, url.Values{"action": {"keep"}}, cookie)
	readBody(t, resp)
	if resp.StatusCode != http.StatusFound || !strings.Contains(resp.Header.Get("Location"), "error=") {
		t.Fatalf("invalid update status = %d", resp.StatusCode)
	}
	resp = postFormRequest(t, srv, "/policies/missing", update, cookie)
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing update status = %d", resp.StatusCode)
	}

	resp = postFormRequest(t, srv, "/policies/"+second.ID+"/toggle", url.Values{}, cookie)
	readBody(t, resp)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("toggle status = %d", resp.StatusCode)
	}
	toggled, _ := store.GetPolicy(context.Background(), second.ID)
	if toggled.Enabled {
		t.Fatal("toggle should disable")
	}
	resp = postFormRequest(t, srv, "/policies/missing/toggle", url.Values{}, cookie)
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing toggle status = %d", resp.StatusCode)
	}

	resp = postFormRequest(t, srv, "/policies/"+first.ID+"/move", url.Values{"dir": {"down"}}, cookie)
	readBody(t, resp)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("move status = %d", resp.StatusCode)
	}
	ordered, _ := store.ListPolicies(context.Background())
	if ordered[0].ID != second.ID || ordered[1].ID != first.ID || ordered[2].ID != third.ID {
		t.Fatalf("order = %+v", ordered)
	}
	resp = postFormRequest(t, srv, "/policies/"+second.ID+"/move", url.Values{"dir": {"up"}}, cookie)
	readBody(t, resp)
	ordered, _ = store.ListPolicies(context.Background())
	if ordered[0].ID != second.ID {
		t.Fatalf("edge move changed order: %+v", ordered)
	}
	resp = postFormRequest(t, srv, "/policies/missing/move", url.Values{"dir": {"up"}}, cookie)
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing move status = %d", resp.StatusCode)
	}

	resp = postFormRequest(t, srv, "/policies/"+third.ID+"/delete", url.Values{}, cookie)
	readBody(t, resp)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	if _, err := store.GetPolicy(context.Background(), third.ID); err == nil {
		t.Fatal("policy should be deleted")
	}
	resp = postFormRequest(t, srv, "/policies/missing/delete", url.Values{}, cookie)
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing delete status = %d", resp.StatusCode)
	}

	types := map[string]bool{}
	for _, event := range bus.Recent(0) {
		types[event.Type] = true
	}
	for _, want := range []string{events.TypePolicyUpdated, events.TypePolicyReordered, events.TypePolicyDeleted} {
		if !types[want] {
			t.Errorf("missing event %s", want)
		}
	}
}

func TestPolicyUITester(t *testing.T) {
	srv, _, _ := newTestServer(t)
	cookie := login(t, srv)
	resp := postFormRequest(t, srv, "/policies/test", url.Values{"field": {"subject"}, "pattern": {"^hi"}, "value": {"hi there"}}, cookie)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Match") {
		t.Fatalf("match = %d %.200s", resp.StatusCode, body)
	}
	resp = postFormRequest(t, srv, "/policies/test", url.Values{"field": {"subject"}, "pattern": {"^nope"}, "value": {"hi"}}, cookie)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "No match") {
		t.Fatalf("no match = %d %.200s", resp.StatusCode, body)
	}
	resp = postFormRequest(t, srv, "/policies/test", url.Values{"field": {"subject"}, "pattern": {"(["}, "value": {"hi"}}, cookie)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "invalid regular expression") {
		t.Fatalf("invalid = %d %.200s", resp.StatusCode, body)
	}
}

func TestConsoleFiltersAndPages(t *testing.T) {
	srv, store, bus := newTestServer(t)
	cookie := login(t, srv)
	seedMessage(t, store, "filter1", "inbox", "example.test")
	seedMessage(t, store, "filter2", "other", "other.test")
	seedPolicy(t, store, &models.Policy{Name: "visible policy", Field: models.FieldAny, Pattern: ".*", Action: models.ActionKeep, Enabled: true})
	bus.Emit(events.New(events.TypeMessageReceived, events.SeverityInfo, map[string]any{"subject": "needle"}))

	resp := request(t, srv, http.MethodGet, "/messages?q=filter&domain=example.test&mailbox=inbox&action=keep&port=1025&page=1&limit=1", nil, cookie, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "filter1") {
		t.Fatalf("messages status = %d", resp.StatusCode)
	}
	resp = request(t, srv, http.MethodGet, "/messages?q=does-not-exist", nil, cookie, map[string]string{"HX-Request": "true"})
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "No messages match") {
		t.Fatalf("empty messages = %d", resp.StatusCode)
	}

	resp = request(t, srv, http.MethodGet, "/events?type=message.received&severity=info&q=needle", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "message.received") {
		t.Fatalf("events status = %d", resp.StatusCode)
	}
	resp = request(t, srv, http.MethodGet, "/policies", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "visible policy") {
		t.Fatalf("policies status = %d", resp.StatusCode)
	}
}

func TestLoginEdgeCases(t *testing.T) {
	srv, _, _ := newTestServer(t)

	resp := request(t, srv, http.MethodGet, "/login?next=/messages", nil, nil, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `value="/messages"`) {
		t.Fatalf("login next = %d", resp.StatusCode)
	}
	cookie := login(t, srv)
	resp = request(t, srv, http.MethodGet, "/login", nil, cookie, nil)
	readBody(t, resp)
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/" {
		t.Fatalf("logged-in login = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	resp = postFormRequest(t, srv, "/login", url.Values{}, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(body, "required") {
		t.Fatalf("empty login = %d", resp.StatusCode)
	}

	form := url.Values{"username": {"admin"}, "password": {"secret123"}, "next": {"//evil.test"}}
	resp = postFormRequest(t, srv, "/login", form, nil)
	readBody(t, resp)
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/" {
		t.Fatalf("bad next = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	req := "/messages?q=x"
	resp = request(t, srv, http.MethodGet, req, nil, nil, nil)
	readBody(t, resp)
	if resp.StatusCode != http.StatusFound || !strings.Contains(resp.Header.Get("Location"), "next=%2Fmessages%3Fq%3Dx") {
		t.Fatalf("redirect = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestPasswordChangeFailures(t *testing.T) {
	srv, _, _ := newTestServer(t)
	cookie := login(t, srv)
	cases := []url.Values{
		{"current": {"secret123"}, "password": {"short"}, "confirm": {"short"}},
		{"current": {"secret123"}, "password": {"long-enough"}, "confirm": {"different"}},
		{"current": {"wrong"}, "password": {"long-enough"}, "confirm": {"long-enough"}},
	}
	for _, form := range cases {
		resp := postFormRequest(t, srv, "/settings/password", form, cookie)
		readBody(t, resp)
		if resp.StatusCode != http.StatusFound || !strings.Contains(resp.Header.Get("Location"), "error=") {
			t.Fatalf("form %v status = %d location = %q", form, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	form := url.Values{"username": {"admin"}, "password": {"secret123"}}
	resp := postFormRequest(t, srv, "/login", form, nil)
	readBody(t, resp)
	if resp.StatusCode != http.StatusFound {
		t.Fatal("original password should still work")
	}
}

func TestHealthStaticAndErrors(t *testing.T) {
	srv, _, _ := newTestServer(t)

	resp := request(t, srv, http.MethodGet, "/healthz", nil, nil, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("health = %d %s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodGet, "/static/app.css", nil, nil, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || len(body) == 0 {
		t.Fatalf("static = %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "css") || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("static headers = %+v", resp.Header)
	}
	for _, path := range []string{"/static/missing", "/static/", "/static/../go.mod"} {
		resp = request(t, srv, http.MethodGet, path, nil, nil, nil)
		readBody(t, resp)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s status = %d", path, resp.StatusCode)
		}
	}

	cookie := login(t, srv)
	resp = request(t, srv, http.MethodGet, "/ws/events", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusUpgradeRequired || !strings.Contains(body, "websocket") {
		t.Fatalf("ws status = %d body = %s", resp.StatusCode, body)
	}
	resp = request(t, srv, http.MethodGet, "/api/unknown", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(body, `"error"`) {
		t.Fatalf("api 404 = %d %s", resp.StatusCode, body)
	}
	resp = request(t, srv, http.MethodGet, "/unknown", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(body, "404") {
		t.Fatalf("page 404 = %d %.200s", resp.StatusCode, body)
	}
}

func TestAPIEdgeCases(t *testing.T) {
	srv, store, _ := newTestServer(t)
	cookie := login(t, srv)
	jsonHeaders := map[string]string{"Content-Type": "application/json"}

	policy := seedPolicy(t, store, &models.Policy{Name: "read", Field: models.FieldAny, Pattern: ".*", Action: models.ActionKeep, Enabled: true})
	resp := request(t, srv, http.MethodGet, "/api/policies/"+policy.ID, nil, cookie, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "read") {
		t.Fatalf("get policy = %d %s", resp.StatusCode, body)
	}
	resp = request(t, srv, http.MethodGet, "/api/policies/missing", nil, cookie, nil)
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get missing = %d", resp.StatusCode)
	}

	resp = request(t, srv, http.MethodPost, "/api/policies", strings.NewReader("{"), cookie, jsonHeaders)
	readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid json = %d", resp.StatusCode)
	}
	resp = request(t, srv, http.MethodPost, "/api/policies", strings.NewReader(`{"name":"","field":"sender","pattern":"x","action":"keep"}`), cookie, jsonHeaders)
	readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid policy = %d", resp.StatusCode)
	}
	resp = request(t, srv, http.MethodPut, "/api/policies/missing", strings.NewReader(`{"name":"x","field":"sender","pattern":"x","action":"keep"}`), cookie, jsonHeaders)
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("put missing = %d", resp.StatusCode)
	}
	resp = request(t, srv, http.MethodPut, "/api/policies/"+policy.ID, strings.NewReader("{"), cookie, jsonHeaders)
	readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("put json = %d", resp.StatusCode)
	}
	resp = request(t, srv, http.MethodPut, "/api/policies/"+policy.ID, strings.NewReader(`{"name":"","field":"sender","pattern":"x","action":"keep"}`), cookie, jsonHeaders)
	readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("put invalid = %d", resp.StatusCode)
	}
	resp = request(t, srv, http.MethodDelete, "/api/policies/missing", nil, cookie, nil)
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete missing = %d", resp.StatusCode)
	}
	resp = request(t, srv, http.MethodDelete, "/api/messages/missing", nil, cookie, nil)
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete message missing = %d", resp.StatusCode)
	}

	second := seedPolicy(t, store, &models.Policy{Name: "second", Field: models.FieldAny, Pattern: ".*", Action: models.ActionKeep, Enabled: true})
	_ = second
	reorderCases := []string{
		`{`,
		`{"ids":[]}`,
		`{"ids":["` + policy.ID + `"]}`,
		`{"ids":["` + policy.ID + `","missing"]}`,
		`{"ids":["` + policy.ID + `","` + policy.ID + `"]}`,
	}
	for _, bodyJSON := range reorderCases {
		resp = request(t, srv, http.MethodPost, "/api/policies/reorder", strings.NewReader(bodyJSON), cookie, jsonHeaders)
		readBody(t, resp)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("reorder %s status = %d", bodyJSON, resp.StatusCode)
		}
	}

	resp = request(t, srv, http.MethodPost, "/api/policies/test", strings.NewReader("{"), cookie, jsonHeaders)
	readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("test json = %d", resp.StatusCode)
	}
	resp = request(t, srv, http.MethodPost, "/api/policies/test", strings.NewReader(`{"field":"nope","pattern":"x","value":"x"}`), cookie, jsonHeaders)
	readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("test field = %d", resp.StatusCode)
	}
}

func TestSSEStream(t *testing.T) {
	srv, _, bus := newTestServer(t)
	cookie := login(t, srv)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.App().Listener(ln) }()
	t.Cleanup(func() {
		_ = srv.Shutdown(context.Background())
		_ = ln.Close()
	})

	request, err := http.NewRequest(http.MethodGet, "http://"+ln.Addr().String()+"/api/events/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(cookie)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := (&http.Client{}).Do(request.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "ready") {
		t.Fatalf("first SSE line = %q", line)
	}
	bus.Emit(newEvent("message.received", "info"))
	foundEvent := false
	for i := 0; i < 10; i++ {
		line, err = reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(line, "event: message.received") {
			foundEvent = true
			break
		}
	}
	if !foundEvent {
		t.Fatal("did not receive message.received SSE event")
	}
	cancel()
	_ = response.Body.Close()
}

func TestStartAndShutdown(t *testing.T) {
	cfg := testConfig(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	cfg.HTTPAddr = "127.0.0.1:" + strconv.Itoa(port)
	srv, _, _ := newTestServerWithConfig(t, cfg)
	done := make(chan error, 1)
	go func() { done <- srv.Start() }()

	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get("http://" + cfg.HTTPAddr + "/healthz")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not start: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after shutdown")
	}
}
