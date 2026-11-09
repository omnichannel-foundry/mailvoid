package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gnrfan/mailvoid/internal/config"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mailvoid.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadTOMLFile(t *testing.T) {
	path := writeConfig(t, `
hostname = "mail.example.test"
scheme = "https"
public_url = ""
domains = ["example.test", "*.mail.test"]

[web]
bind = "127.0.0.1"
port = 9000

[smtp]
hosts = ["127.0.0.1"]
ports = [1025, 2525]
domains = ["example.test"]

[[smtp.services]]
name = "dev"
host = "127.0.0.1"
port = 1025
aliases = ["local", "development"]

[[smtp.services]]
name = "qa"
host = "0.0.0.0"
port = 2525
aliases = ["staging"]

[events]
json_enabled = false
json_path = "/tmp/events.jsonl"

[logging]
enabled = true
dir = "/tmp/mailvoid-logs"

[storage]
data_dir = "/tmp/mailvoid-data"
db_path = "/tmp/mailvoid-data/custom.db"

[search]
enabled = false
index_path = "/tmp/mailvoid-data/custom.bleve"

[auth]
admin_user = "root"
admin_password = "secret"
api_token = "token"
webdav_user = "dav"
webdav_password = "dav-secret"
cookie_secure = true

[limits]
max_message_bytes = 1024
read_timeout = "5s"
sweep_interval = "1m"
`)
	cfg, err := config.Load([]string{"-config", path}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hostname != "mail.example.test" || cfg.Scheme != "https" {
		t.Fatalf("identity = %q %q", cfg.Hostname, cfg.Scheme)
	}
	if cfg.PublicBaseURL() != "https://mail.example.test:9000" {
		t.Fatalf("public url = %q", cfg.PublicBaseURL())
	}
	if cfg.HTTPAddr != "127.0.0.1:9000" {
		t.Fatalf("http addr = %q", cfg.HTTPAddr)
	}
	if len(cfg.SMTPServices) != 2 {
		t.Fatalf("services = %+v", cfg.SMTPServices)
	}
	if cfg.SMTPServices[0].Name != "dev" || cfg.SMTPServices[0].LogName() != "dev" {
		t.Fatalf("dev service = %+v", cfg.SMTPServices[0])
	}
	if cfg.SMTPServices[1].Aliases[0] != "staging" {
		t.Fatalf("aliases = %+v", cfg.SMTPServices[1].Aliases)
	}
	if cfg.EventLogEnabled() || cfg.EventLogPath != "/tmp/events.jsonl" {
		t.Fatalf("event log = %v %q", cfg.EventLogEnabled(), cfg.EventLogPath)
	}
	if !cfg.LogEnabled || cfg.LogDir != "/tmp/mailvoid-logs" {
		t.Fatalf("logging = %v %q", cfg.LogEnabled, cfg.LogDir)
	}
	if cfg.DBPath != "/tmp/mailvoid-data/custom.db" || cfg.SearchEnabled {
		t.Fatalf("storage = %q search=%v", cfg.DBPath, cfg.SearchEnabled)
	}
	if cfg.AdminUser != "root" || cfg.APIToken != "token" || !cfg.SessionCookieSecure {
		t.Fatalf("auth = %+v", cfg)
	}
	if cfg.MaxMessageBytes != 1024 || cfg.ReadTimeout.String() != "5s" || cfg.SweepInterval.String() != "1m0s" {
		t.Fatalf("limits = %+v", cfg)
	}
	if !cfg.AllowsDomain("example.test") || cfg.AllowsDomain("other.test") {
		t.Fatal("domains should follow the smtp section")
	}
}

func TestLoadDefaultConfigFromWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mailvoid.toml"), []byte("hostname = \"from-file.test\"\nscheme = \"https\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	cfg, err := config.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hostname != "from-file.test" || cfg.Scheme != "https" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestConfigPrecedence(t *testing.T) {
	path := writeConfig(t, `
hostname = "file.test"
scheme = "http"

[web]
port = 7000

[smtp]
ports = [2025]
`)
	env := map[string]string{
		"MAILVOID_HOSTNAME":   "env.test",
		"MAILVOID_WEB_PORT":   "8000",
		"MAILVOID_SMTP_PORTS": "3025",
	}
	cfg, err := config.Load([]string{"-config", path, "-hostname", "flag.test", "-web-port", "9000"}, func(k string) string {
		return env[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hostname != "flag.test" || cfg.HTTPAddr != ":9000" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if len(cfg.SMTPPorts) != 1 || cfg.SMTPPorts[0] != 3025 {
		t.Fatalf("ports = %v", cfg.SMTPPorts)
	}
}

func TestConfigErrors(t *testing.T) {
	if _, err := config.Load([]string{"-config", filepath.Join(t.TempDir(), "missing.toml")}, nil); err == nil {
		t.Fatal("expected missing explicit config error")
	}
	path := writeConfig(t, "unknown_key = true\n")
	if _, err := config.Load([]string{"-config", path}, nil); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected unknown key error, got %v", err)
	}

	path = writeConfig(t, `
[[smtp.services]]
name = "same"
host = "127.0.0.1"
port = 1025

[[smtp.services]]
name = "same"
host = "127.0.0.1"
port = 2525
`)
	if _, err := config.Load([]string{"-config", path}, nil); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("expected duplicate service error, got %v", err)
	}
}

func TestPublicBaseURL(t *testing.T) {
	cfg := config.Default()
	cfg.HTTPAddr = ":8025"
	if got := cfg.PublicBaseURL(); got != "http://localhost:8025" {
		t.Fatalf("url = %q", got)
	}
	cfg.Scheme = "https"
	cfg.HTTPAddr = ":443"
	if got := cfg.PublicBaseURL(); got != "https://localhost" {
		t.Fatalf("url = %q", got)
	}
	cfg.PublicURL = "https://public.example.test/"
	if got := cfg.PublicBaseURL(); got != "https://public.example.test" {
		t.Fatalf("url = %q", got)
	}
}
