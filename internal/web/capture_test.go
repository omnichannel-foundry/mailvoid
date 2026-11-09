package web_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gnrfan/mailvoid/internal/capture"
	"github.com/gnrfan/mailvoid/internal/events"
)

func TestCaptureToggle(t *testing.T) {
	srv, _, bus := newTestServer(t)
	controller := capture.New(true)
	srv.SetCaptureController(controller)
	cookie := login(t, srv)

	resp := request(t, srv, http.MethodGet, "/settings", nil, cookie, nil)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `name="enabled"`) || !strings.Contains(body, "checked") {
		t.Fatalf("settings page should show capture on: status=%d body=%s", resp.StatusCode, body)
	}

	resp = request(t, srv, http.MethodPost, "/settings/capture", strings.NewReader(url.Values{}.Encode()), cookie,
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusFound || !strings.Contains(resp.Header.Get("Location"), "Capture+disabled") {
		t.Fatalf("disable status=%d location=%q body=%s", resp.StatusCode, resp.Header.Get("Location"), body)
	}
	if controller.Enabled() {
		t.Fatal("capture should be disabled")
	}
	if !recentEvent(bus, events.TypeSystemCapture, false) {
		t.Fatal("missing capture disabled event")
	}

	resp = request(t, srv, http.MethodGet, "/settings", nil, cookie, nil)
	body = readBody(t, resp)
	if !strings.Contains(body, "Capture is paused") {
		t.Fatalf("settings page should warn capture is paused: %s", body)
	}

	resp = request(t, srv, http.MethodPost, "/settings/capture",
		strings.NewReader(url.Values{"enabled": {"1"}}.Encode()), cookie,
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusFound || !strings.Contains(resp.Header.Get("Location"), "Capture+enabled") {
		t.Fatalf("enable status=%d location=%q body=%s", resp.StatusCode, resp.Header.Get("Location"), body)
	}
	if !controller.Enabled() {
		t.Fatal("capture should be enabled")
	}
	if !recentEvent(bus, events.TypeSystemCapture, true) {
		t.Fatal("missing capture enabled event")
	}
}

func recentEvent(bus *events.Bus, typ string, enabled bool) bool {
	for _, event := range bus.Recent(0) {
		if event.Type == typ && event.Payload["enabled"] == enabled {
			return true
		}
	}
	return false
}
