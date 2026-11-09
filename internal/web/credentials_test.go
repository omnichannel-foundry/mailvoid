package web_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/events"
)

type createdKey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	Token     string     `json:"token"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func TestManagedAPIKeys(t *testing.T) {
	srv, _, bus := newTestServer(t)
	cookie := login(t, srv)
	jsonHeaders := map[string]string{"Content-Type": "application/json"}

	resp := request(t, srv, http.MethodPost, "/api/v1/admin/api-keys",
		strings.NewReader(`{"name":"it-agent"}`), cookie, jsonHeaders)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", resp.StatusCode, body)
	}
	var created createdKey
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Token == "" || !strings.HasPrefix(created.Token, "mv_") {
		t.Fatalf("created = %+v", created)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/admin/api-keys", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, created.ID) || strings.Contains(body, created.Token) {
		t.Fatalf("list status = %d body = %s", resp.StatusCode, body)
	}

	managedHeaders := map[string]string{"Authorization": "Bearer " + created.Token}
	resp = request(t, srv, http.MethodGet, "/api/v1/stats", nil, nil, managedHeaders)
	readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("managed auth status = %d", resp.StatusCode)
	}
	resp = request(t, srv, http.MethodGet, "/api/v1/admin/api-keys", nil, nil, managedHeaders)
	readBody(t, resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("managed admin status = %d", resp.StatusCode)
	}

	resp = request(t, srv, http.MethodPost, "/api/v1/admin/api-keys", strings.NewReader(`{"name":""}`), cookie, jsonHeaders)
	readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name status = %d", resp.StatusCode)
	}
	resp = request(t, srv, http.MethodPost, "/api/v1/admin/api-keys", strings.NewReader(`{"name":"x","expires_at":"soon"}`), cookie, jsonHeaders)
	readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad expiry status = %d", resp.StatusCode)
	}

	resp = request(t, srv, http.MethodDelete, "/api/v1/admin/api-keys/"+created.ID, nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"deleted":true`) {
		t.Fatalf("delete status = %d body = %s", resp.StatusCode, body)
	}
	resp = request(t, srv, http.MethodGet, "/api/v1/stats", nil, nil, managedHeaders)
	readBody(t, resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked key status = %d", resp.StatusCode)
	}
	resp = request(t, srv, http.MethodDelete, "/api/v1/admin/api-keys/missing", nil, cookie, nil)
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing delete status = %d", resp.StatusCode)
	}

	foundCreated := false
	for _, event := range bus.Recent(0) {
		if event.Type == events.TypeAPIKeyCreated || event.Type == events.TypeAPIKeyDeleted {
			foundCreated = true
		}
	}
	if !foundCreated {
		t.Fatal("expected API key events")
	}
}

func TestBootstrapTokenCanManageCredentials(t *testing.T) {
	srv, _, _ := newTestServer(t)
	resp := request(t, srv, http.MethodGet, "/api/v1/admin/api-keys", nil, nil, map[string]string{"Authorization": "Bearer " + testAPIToken})
	readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap token status = %d", resp.StatusCode)
	}
}

func TestManagedWebDAVUsers(t *testing.T) {
	srv, store, bus := newTestServer(t)
	cookie := login(t, srv)
	seedDAVMessages(t, store)
	jsonHeaders := map[string]string{"Content-Type": "application/json"}

	resp := request(t, srv, http.MethodPost, "/api/v1/admin/webdav-users",
		strings.NewReader(`{"username":"agent","password":"long-enough-password"}`), cookie, jsonHeaders)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", resp.StatusCode, body)
	}
	var created struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		Generated bool   `json:"generated"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatal(err)
	}
	if created.Username != "agent" || created.Password != "long-enough-password" || created.Generated {
		t.Fatalf("created = %+v", created)
	}

	resp = request(t, srv, "PROPFIND", "/dav/", strings.NewReader(propfindBody), nil, map[string]string{
		"Authorization": basicAuth("agent", "long-enough-password"),
		"Depth":         "1",
	})
	readBody(t, resp)
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("webdav managed status = %d", resp.StatusCode)
	}

	resp = request(t, srv, http.MethodGet, "/api/v1/admin/webdav-users", nil, cookie, nil)
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "agent") {
		t.Fatalf("list status = %d body = %s", resp.StatusCode, body)
	}

	if resp = request(t, srv, http.MethodPost, "/api/v1/admin/webdav-users",
		strings.NewReader(`{"username":"short","password":"short"}`), cookie, jsonHeaders); resp.StatusCode != http.StatusBadRequest {
		readBody(t, resp)
		t.Fatalf("short password status = %d", resp.StatusCode)
	} else {
		readBody(t, resp)
	}
	if resp = request(t, srv, http.MethodPost, "/api/v1/admin/webdav-users",
		strings.NewReader(`{"username":"dav","password":"long-enough-password"}`), cookie, jsonHeaders); resp.StatusCode != http.StatusConflict {
		readBody(t, resp)
		t.Fatalf("reserved username status = %d", resp.StatusCode)
	} else {
		readBody(t, resp)
	}

	resp = request(t, srv, http.MethodDelete, "/api/v1/admin/webdav-users/agent", nil, cookie, nil)
	readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	resp = request(t, srv, "PROPFIND", "/dav/", strings.NewReader(propfindBody), nil, map[string]string{
		"Authorization": basicAuth("agent", "long-enough-password"),
		"Depth":         "1",
	})
	readBody(t, resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("deleted webdav status = %d", resp.StatusCode)
	}
	resp = request(t, srv, http.MethodDelete, "/api/v1/admin/webdav-users/missing", nil, cookie, nil)
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing delete status = %d", resp.StatusCode)
	}

	foundCreated := false
	for _, event := range bus.Recent(0) {
		if event.Type == events.TypeWebDAVUserCreated || event.Type == events.TypeWebDAVUserDeleted {
			foundCreated = true
		}
	}
	if !foundCreated {
		t.Fatal("expected WebDAV user events")
	}
}

func TestManagedWebDAVEnablesSurface(t *testing.T) {
	cfg := testConfig(t)
	cfg.WebDAVPassword = ""
	cfg.WebDAVUser = ""
	srv, _, _ := newTestServerWithConfig(t, cfg)
	cookie := login(t, srv)
	jsonHeaders := map[string]string{"Content-Type": "application/json"}

	resp := request(t, srv, "PROPFIND", "/dav/", strings.NewReader(propfindBody), nil, map[string]string{"Depth": "1"})
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("before create status = %d", resp.StatusCode)
	}

	resp = request(t, srv, http.MethodPost, "/api/v1/admin/webdav-users",
		strings.NewReader(`{"username":"generated"}`), cookie, jsonHeaders)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", resp.StatusCode, body)
	}
	var created struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		Generated bool   `json:"generated"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatal(err)
	}
	if !created.Generated || len(created.Password) < 12 {
		t.Fatalf("generated = %+v", created)
	}
	resp = request(t, srv, "PROPFIND", "/dav/", strings.NewReader(propfindBody), nil, map[string]string{
		"Authorization": basicAuth(created.Username, created.Password),
		"Depth":         "1",
	})
	readBody(t, resp)
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("after create webdav status = %d", resp.StatusCode)
	}
}
