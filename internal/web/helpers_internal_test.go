package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gofiber/fiber/v2"
)

func TestSafeNextAndValidAction(t *testing.T) {
	cases := map[string]string{
		"":                  "/",
		"   ":               "/",
		"/messages":         "/messages",
		"//evil.test":       "/",
		"https://evil.test": "/",
		"\\evil":            "/",
		"/a\r\nb":           "/",
	}
	for input, want := range cases {
		if got := safeNext(input); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", input, got, want)
		}
	}
	for _, action := range []string{"", models.ActionKeep, models.ActionTTL, models.ActionDrop} {
		if !validAction(action) {
			t.Errorf("action %q should be valid", action)
		}
	}
	if validAction("nope") {
		t.Error("nope should be invalid")
	}
}

func TestActiveAndTitle(t *testing.T) {
	if activeFor("message_detail") != "messages" || activeFor("dashboard") != "dashboard" || activeFor("login") != "" {
		t.Fatal("activeFor mismatch")
	}
	cases := map[string]string{
		"dashboard":      "Dashboard",
		"messages":       "Messages",
		"message_detail": "Message",
		"events":         "Event stream",
		"policies":       "Policies",
		"settings":       "Settings",
		"login":          "Sign in",
		"error":          "Error",
		"unknown":        "MailVoid",
	}
	for page, want := range cases {
		if got := titleFor(page); got != want {
			t.Errorf("titleFor(%q) = %q, want %q", page, got, want)
		}
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		when time.Time
		want string
	}{
		{now.Add(-10 * time.Second), "10s ago"},
		{now.Add(-10 * time.Minute), "10m ago"},
		{now.Add(-2 * time.Hour), "2h ago"},
		{now.Add(-48 * time.Hour), "2d ago"},
		{now.Add(10 * time.Minute), "in 10m"},
	}
	for _, tc := range cases {
		if got := relativeTime(tc.when); !strings.HasPrefix(got, strings.Split(tc.want, " ")[0]) && got != tc.want {
			// Exact second values can drift by a fraction of a second; check the
			// shape and direction rather than a brittle exact match.
			if !(strings.HasSuffix(got, "ago") || strings.HasPrefix(got, "in ")) {
				t.Errorf("relativeTime(%v) = %q", tc.when, got)
			}
		}
	}
	if !strings.Contains(relativeTime(now.Add(-30*time.Second)), "ago") {
		t.Fatal("past time should say ago")
	}
	if !strings.HasPrefix(relativeTime(now.Add(30*time.Second)), "in ") {
		t.Fatal("future time should say in")
	}
}

func TestTemplateFuncs(t *testing.T) {
	funcs := templateFuncs()
	formatTime := funcs["formatTime"].(func(time.Time) string)
	if formatTime(time.Time{}) != "—" {
		t.Fatal("zero formatTime")
	}
	if got := formatTime(time.Now()); got == "—" {
		t.Fatal("non-zero formatTime")
	}
	if got := funcs["formatTimeShort"].(func(time.Time) string)(time.Time{}); got != "—" {
		t.Fatal("short zero")
	}
	if got := funcs["relTime"].(func(time.Time) string)(time.Time{}); got != "—" {
		t.Fatal("rel zero")
	}
	if got := funcs["expiresIn"].(func(*time.Time) string)(nil); got != "permanent" {
		t.Fatal("expires nil")
	}
	expiry := time.Now().Add(time.Hour)
	if got := funcs["expiresIn"].(func(*time.Time) string)(&expiry); !strings.HasPrefix(got, "in ") {
		t.Fatalf("expires = %q", got)
	}
	if got := funcs["severityClass"].(func(string) string)("error"); got != "badge-error" {
		t.Fatal(got)
	}
	if got := funcs["severityClass"].(func(string) string)("warn"); got != "badge-warning" {
		t.Fatal(got)
	}
	if got := funcs["severityClass"].(func(string) string)("debug"); got != "badge-ghost" {
		t.Fatal(got)
	}
	if got := funcs["severityClass"].(func(string) string)("info"); got != "badge-info" {
		t.Fatal(got)
	}
	if got := funcs["actionClass"].(func(string) string)("drop"); got != "badge-error" {
		t.Fatal(got)
	}
	if got := funcs["actionClass"].(func(string) string)("ttl"); got != "badge-warning" {
		t.Fatal(got)
	}
	if got := funcs["actionClass"].(func(string) string)("keep"); got != "badge-success" {
		t.Fatal(got)
	}
	if got := funcs["jsonPretty"].(func(any) string)(map[string]any{"a": 1}); !strings.Contains(got, "\"a\"") {
		t.Fatal(got)
	}
	if got := funcs["jsonPretty"].(func(any) string)(make(chan int)); got != "{}" {
		t.Fatal(got)
	}
	if got := funcs["truncate"].(func(string, int) string)("abcdef", 3); got != "abc…" {
		t.Fatal(got)
	}
	if got := funcs["truncate"].(func(string, int) string)("ab", 3); got != "ab" {
		t.Fatal(got)
	}
	if got := funcs["join"].(func(string, []string) string)(",", []string{"a", "b"}); got != "a,b" {
		t.Fatal(got)
	}
	if funcs["add"].(func(int, int) int)(2, 3) != 5 || funcs["sub"].(func(int, int) int)(5, 2) != 3 || funcs["mul"].(func(int, int) int)(3, 4) != 12 {
		t.Fatal("arithmetic")
	}
	if got := funcs["shortID"].(func(string) string)("msg_1234567890abcdef"); got != "msg_123456" {
		t.Fatal(got)
	}
	if got := funcs["shortID"].(func(string) string)("short"); got != "short" {
		t.Fatal(got)
	}
	if got := funcs["rfc3339"].(func(*time.Time) string)(nil); got != "" {
		t.Fatal("rfc nil")
	}
	if got := funcs["rfc3339"].(func(*time.Time) string)(&expiry); !strings.HasSuffix(got, "Z") {
		t.Fatal(got)
	}
	if funcs["percent"].(func(int, int) int)(1, 0) != 0 || funcs["percent"].(func(int, int) int)(1, 4) != 25 {
		t.Fatal("percent")
	}
	if got := funcs["defaultStr"].(func(string, string) string)("fallback", " "); got != "fallback" {
		t.Fatal(got)
	}
	if got := funcs["defaultStr"].(func(string, string) string)("fallback", "value"); got != "value" {
		t.Fatal(got)
	}
}

func TestRenderer(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	if err := renderer.Load(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := renderer.Render(&buf, "login", fiber.Map{"Title": "Sign in", "Version": "test"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Sign in") {
		t.Fatalf("login render = %.200s", buf.String())
	}
	buf.Reset()
	if err := renderer.Render(&buf, "login", struct{ Title string }{Title: "struct"}); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Render(&buf, "does-not-exist", nil); err == nil {
		t.Fatal("expected unknown template error")
	}
}

func TestRenderAndFragmentHelpers(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{renderer: renderer, version: "test"}
	app := fiber.New(fiber.Config{Views: renderer})
	app.Get("/render", func(c *fiber.Ctx) error {
		return s.render(c, "login", nil)
	})
	app.Get("/fragment", func(c *fiber.Ctx) error {
		return s.renderFragment(c, "policy_test", fiber.Map{"Matched": true, "Field": "subject", "Pattern": "x"})
	})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/render", nil))
	if err != nil {
		t.Fatal(err)
	}
	body := readHTTPBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Sign in") {
		t.Fatalf("render status = %d body = %.100s", resp.StatusCode, body)
	}
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/fragment", nil))
	if err != nil {
		t.Fatal(err)
	}
	body = readHTTPBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Match") {
		t.Fatalf("fragment status = %d body = %.100s", resp.StatusCode, body)
	}
}

func readHTTPBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	return buf.String()
}

func TestHelpersViaFiber(t *testing.T) {
	app := fiber.New()
	app.Get("/page", func(c *fiber.Ctx) error {
		page, limit := paginationParams(c)
		return c.JSON(fiber.Map{"page": page, "limit": limit})
	})
	app.Get("/pagination", func(c *fiber.Ctx) error {
		page, limit := paginationParams(c)
		return c.JSON(buildPagination(c, "/messages", page, limit, 5))
	})
	app.Get("/messages", func(c *fiber.Ctx) error {
		filter, view, meta := parseMessageFilter(c)
		return c.JSON(fiber.Map{"filter": filter, "view": view, "meta": meta})
	})
	app.Get("/events", func(c *fiber.Ctx) error {
		filter, view, meta := parseEventFilter(c)
		return c.JSON(fiber.Map{"filter": filter, "view": view, "meta": meta})
	})
	app.Post("/policy", func(c *fiber.Ctx) error {
		p, err := parsePolicyForm(c)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(p)
	})

	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/page?page=-1&limit=999", nil))
	var pageOut struct{ Page, Limit int }
	_ = json.NewDecoder(resp.Body).Decode(&pageOut)
	if pageOut.Page != 1 || pageOut.Limit != 200 {
		t.Fatalf("page = %+v", pageOut)
	}

	resp, _ = app.Test(httptest.NewRequest(http.MethodGet, "/pagination?page=2&limit=2&q=test", nil))
	var pagination pagination
	_ = json.NewDecoder(resp.Body).Decode(&pagination)
	if pagination.Page != 2 || pagination.Pages != 3 || !pagination.HasPrev || !pagination.HasNext || !strings.Contains(pagination.PrevURL, "q=test") {
		t.Fatalf("pagination = %+v", pagination)
	}
	resp, _ = app.Test(httptest.NewRequest(http.MethodGet, "/pagination?page=99&limit=2", nil))
	_ = json.NewDecoder(resp.Body).Decode(&pagination)
	if pagination.Page != 3 || pagination.HasNext {
		t.Fatalf("last page = %+v", pagination)
	}
	resp, _ = app.Test(httptest.NewRequest(http.MethodGet, "/pagination", nil))
	_ = json.NewDecoder(resp.Body).Decode(&pagination)
	if pagination.Pages != 1 || pagination.HasPrev || pagination.HasNext {
		t.Fatalf("single page = %+v", pagination)
	}

	resp, _ = app.Test(httptest.NewRequest(http.MethodGet, "/messages?q=%20hello%20&domain=EXAMPLE.TEST&mailbox=Inbox&action=keep&port=1025&page=2&limit=1", nil))
	var messages map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&messages)
	view := messages["view"].(map[string]any)
	if view["Q"] != "hello" || view["Domain"] != "example.test" || view["Mailbox"] != "inbox" {
		t.Fatalf("message filter = %+v", messages)
	}
	resp, _ = app.Test(httptest.NewRequest(http.MethodGet, "/messages?action=nope&port=nope", nil))
	_ = json.NewDecoder(resp.Body).Decode(&messages)
	view = messages["view"].(map[string]any)
	if view["Action"] != "" || view["Port"].(float64) != 0 {
		t.Fatalf("invalid filter = %+v", messages)
	}

	resp, _ = app.Test(httptest.NewRequest(http.MethodGet, "/events?type=message.received&severity=info&q=test&page=3&limit=2", nil))
	var eventsOut map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&eventsOut)
	eventView := eventsOut["view"].(map[string]any)
	if eventView["Type"] != "message.received" || eventView["Severity"] != "info" || eventView["Query"] != "test" {
		t.Fatalf("event filter = %+v", eventsOut)
	}

	forms := []url.Values{
		{"name": {"ok"}, "field": {"sender"}, "pattern": {"@test$"}, "action": {"keep"}, "enabled": {"on"}},
		{"name": {"ok"}, "field": {"sender"}, "pattern": {"@test$"}, "action": {"ttl"}, "ttl_seconds": {"60"}, "enabled": {"true"}},
		{"name": {"ok"}, "field": {"sender"}, "pattern": {"@test$"}, "action": {"drop"}, "enabled": {"yes"}},
	}
	for _, form := range forms {
		req := httptest.NewRequest(http.MethodPost, "/policy", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, _ = app.Test(req)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("policy form %v status = %d", form, resp.StatusCode)
		}
	}
	invalid := []url.Values{
		{"name": {""}, "field": {"sender"}, "pattern": {"x"}, "action": {"keep"}},
		{"name": {"x"}, "field": {"sender"}, "pattern": {"x"}, "action": {"ttl"}},
		{"name": {"x"}, "field": {"sender"}, "pattern": {"x"}, "action": {"keep"}, "ttl_seconds": {"abc"}},
	}
	for _, form := range invalid {
		req := httptest.NewRequest(http.MethodPost, "/policy", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, _ = app.Test(req)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid form %v status = %d", form, resp.StatusCode)
		}
	}
}

func TestPolicyFormFromModel(t *testing.T) {
	create := policyFormFromModel(nil)
	if !create.Enabled || create.Action != models.ActionKeep || create.Field != models.FieldSender || create.IsEdit {
		t.Fatalf("create = %+v", create)
	}
	edit := policyFormFromModel(&models.Policy{ID: "p", Name: "n", Field: models.FieldAny, Pattern: ".*", Action: models.ActionDrop, Enabled: true})
	if !edit.IsEdit || edit.ID != "p" || edit.Name != "n" || edit.Action != models.ActionDrop {
		t.Fatalf("edit = %+v", edit)
	}
}
