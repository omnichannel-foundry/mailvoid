package smtpserver_test

import (
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gnrfan/mailvoid/internal/config"
)

func TestPerServiceLog(t *testing.T) {
	cfg := testConfig(0)
	cfg.LogEnabled = true
	cfg.LogDir = t.TempDir()
	cfg.SMTPServices = []config.SMTPService{{
		Name:    "dev",
		Host:    "127.0.0.1",
		Port:    0,
		Aliases: []string{"local", "development"},
	}}

	srv, _, _ := startServer(t, cfg)
	addr := srv.Addrs()[0].String()
	if err := smtp.SendMail(addr, nil, "sender@example.org", []string{"inbox@example.test"}, []byte(sampleMessage)); err != nil {
		t.Fatalf("send: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(cfg.LogDir, "dev.log"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{"[dev]", "listening addr=", "connect remote=", "mail-from", "rcpt-to accepted", "message received", "disconnect"} {
		if !strings.Contains(text, want) {
			t.Errorf("log missing %q:\n%s", want, text)
		}
	}
}
