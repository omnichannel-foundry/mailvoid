package web_test

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/models"
)

func basicAuth(user, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))
}

func davHeaders() map[string]string {
	return map[string]string{
		"Authorization": basicAuth("dav", "dav-secret"),
		"Depth":         "1",
		"Content-Type":  "application/xml",
	}
}

const propfindBody = `<?xml version="1.0"?><propfind xmlns="DAV:"><allprop/></propfind>`

func seedDAVMessages(t *testing.T, store interface {
	SaveMessage(context.Context, *models.Message) error
}) {
	t.Helper()
	now := time.Now().UTC()
	for _, message := range []*models.Message{
		{ID: "dav1", Mailbox: "inbox", Domain: "example.test", Recipient: "inbox@example.test", Sender: "a@b.test", Subject: "First", Raw: "dav raw one", ReceivedAt: now.Add(-2 * time.Minute), Action: models.ActionKeep},
		{ID: "dav2", Mailbox: "inbox", Domain: "example.test", Recipient: "inbox@example.test", Sender: "b@b.test", Subject: "Second", Raw: "dav raw two", ReceivedAt: now.Add(-1 * time.Minute), Action: models.ActionKeep},
		{ID: "dav3", Mailbox: "other", Domain: "other.test", Recipient: "other@other.test", Sender: "c@b.test", Subject: "Third", Raw: "dav raw three", ReceivedAt: now, Action: models.ActionKeep},
	} {
		if err := store.SaveMessage(context.Background(), message); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWebDAVAuthAndReadOnly(t *testing.T) {
	srv, store, _ := newTestServer(t)
	seedDAVMessages(t, store)

	resp := request(t, srv, "PROPFIND", "/dav/", strings.NewReader(propfindBody), nil, map[string]string{"Depth": "1"})
	readBody(t, resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("WWW-Authenticate"), "Basic") {
		t.Fatalf("challenge = %q", resp.Header.Get("WWW-Authenticate"))
	}

	resp = request(t, srv, "PROPFIND", "/dav/", strings.NewReader(propfindBody), nil, map[string]string{
		"Authorization": basicAuth("dav", "wrong"),
		"Depth":         "1",
	})
	readBody(t, resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d", resp.StatusCode)
	}

	resp = request(t, srv, http.MethodPut, "/dav/inbox@example.test/new.eml", strings.NewReader("x"), nil, davHeaders())
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("put status = %d, body = %s", resp.StatusCode, body)
	}
	if allow := resp.Header.Get("Allow"); !strings.Contains(allow, "PROPFIND") {
		t.Fatalf("allow = %q", allow)
	}
}

func TestWebDAVListingAndDownload(t *testing.T) {
	srv, store, _ := newTestServer(t)
	seedDAVMessages(t, store)

	resp := request(t, srv, "PROPFIND", "/dav/", strings.NewReader(propfindBody), nil, davHeaders())
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("root status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "inbox@example.test") || !strings.Contains(body, "other@other.test") {
		t.Fatalf("root listing = %s", body)
	}

	resp = request(t, srv, "PROPFIND", "/dav/inbox@example.test/", strings.NewReader(propfindBody), nil, davHeaders())
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("mailbox status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "dav1.eml") || !strings.Contains(body, "dav2.eml") {
		t.Fatalf("mailbox listing = %s", body)
	}

	resp = request(t, srv, http.MethodGet, "/dav/inbox@example.test/dav2.eml", nil, nil, davHeaders())
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || body != "dav raw two" {
		t.Fatalf("download status = %d, body = %q", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "message/rfc822") {
		t.Fatalf("content-type = %q", ct)
	}

	resp = request(t, srv, http.MethodHead, "/dav/inbox@example.test/dav1.eml", nil, nil, davHeaders())
	readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("head status = %d", resp.StatusCode)
	}

	resp = request(t, srv, http.MethodGet, "/dav/inbox@example.test/dav3.eml", nil, nil, davHeaders())
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-mailbox status = %d", resp.StatusCode)
	}

	resp = request(t, srv, http.MethodGet, "/dav/inbox@example.test/missing.eml", nil, nil, davHeaders())
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d", resp.StatusCode)
	}
}

func TestWebDAVDisabled(t *testing.T) {
	cfg := testConfig(t)
	cfg.WebDAVPassword = ""
	srv, _, _ := newTestServerWithConfig(t, cfg)
	resp := request(t, srv, "PROPFIND", "/dav/", strings.NewReader(propfindBody), nil, davHeaders())
	readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("disabled status = %d", resp.StatusCode)
	}
}

func TestWebDAVXMLShape(t *testing.T) {
	srv, store, _ := newTestServer(t)
	seedDAVMessages(t, store)
	resp := request(t, srv, "PROPFIND", "/dav/", strings.NewReader(propfindBody), nil, davHeaders())
	body := readBody(t, resp)
	var multistatus struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal([]byte(body), &multistatus); err != nil {
		t.Fatalf("invalid XML: %v (%s)", err, body)
	}
	if multistatus.XMLName.Local != "multistatus" {
		t.Fatalf("root element = %q", multistatus.XMLName.Local)
	}
}
