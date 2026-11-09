package web_test

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gnrfan/mailvoid/internal/events"
)

func TestWebSocketEventStream(t *testing.T) {
	srv, _, bus := newTestServer(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.App().Listener(ln) }()
	t.Cleanup(func() {
		_ = srv.App().Shutdown()
		_ = ln.Close()
	})

	base := "http://" + ln.Addr().String()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	resp, err := client.PostForm(base+"/login", url.Values{
		"username": {"admin"},
		"password": {"secret123"},
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", resp.StatusCode)
	}

	u, _ := url.Parse(base)
	header := http.Header{}
	for _, cookie := range jar.Cookies(u) {
		header.Add("Cookie", cookie.String())
	}

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, wsResp, err := dialer.Dial("ws://"+ln.Addr().String()+"/ws/events", header)
	if err != nil {
		status := 0
		if wsResp != nil {
			status = wsResp.StatusCode
		}
		t.Fatalf("dial: %v (status %d)", err, status)
	}
	defer conn.Close()

	time.Sleep(150 * time.Millisecond)
	bus.Emit(events.New(events.TypeSystemStartup, events.SeverityInfo, map[string]any{"test": true}))

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var ev events.Event
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatalf("decode: %v (%s)", err, data)
	}
	if ev.Type != events.TypeSystemStartup {
		t.Fatalf("event = %+v", ev)
	}
	if ev.Payload["test"] != true {
		t.Fatalf("payload = %+v", ev.Payload)
	}
}
