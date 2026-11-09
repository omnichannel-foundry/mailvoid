package config_test

import (
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/config"
)

func TestDefaults(t *testing.T) {
	cfg, err := config.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SMTPPorts) != 2 || cfg.SMTPPorts[0] != 1025 || cfg.SMTPPorts[1] != 2525 {
		t.Fatalf("ports = %v", cfg.SMTPPorts)
	}
	if cfg.HTTPAddr != ":8025" {
		t.Fatalf("http = %q", cfg.HTTPAddr)
	}
	if cfg.DBPath != "data/mailvoid.db" {
		t.Fatalf("db path = %q", cfg.DBPath)
	}
	if cfg.EventLogPath != "data/events.jsonl" {
		t.Fatalf("event log = %q", cfg.EventLogPath)
	}
	if !cfg.SearchEnabled || cfg.SearchIndexPath != "data/search.bleve" {
		t.Fatalf("search = %v %q", cfg.SearchEnabled, cfg.SearchIndexPath)
	}
	if !cfg.AllowsDomain("anything.example") {
		t.Fatal("default should be wildcard")
	}
}

func TestEnvAndFlags(t *testing.T) {
	env := map[string]string{
		"MAILVOID_SMTP_PORTS":        "2025, 3025,2025",
		"MAILVOID_HTTP_ADDR":         ":9000",
		"MAILVOID_DOMAINS":           "example.com, *.example.com, *.test",
		"MAILVOID_ADMIN_USER":        "root",
		"MAILVOID_ADMIN_PASSWORD":    "secret",
		"MAILVOID_SWEEP_INTERVAL":    "5s",
		"MAILVOID_MAX_MESSAGE_BYTES": "1024",
	}
	cfg, err := config.Load([]string{"-http-addr", ":9100", "-cookie-secure"}, func(k string) string {
		return env[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SMTPPorts) != 2 || cfg.SMTPPorts[0] != 2025 || cfg.SMTPPorts[1] != 3025 {
		t.Fatalf("ports = %v", cfg.SMTPPorts)
	}
	if cfg.HTTPAddr != ":9100" {
		t.Fatalf("flag should override env: %q", cfg.HTTPAddr)
	}
	if cfg.AdminUser != "root" || cfg.AdminPassword != "secret" {
		t.Fatalf("admin = %q/%q", cfg.AdminUser, cfg.AdminPassword)
	}
	if cfg.SweepInterval != 5*time.Second || cfg.MaxMessageBytes != 1024 {
		t.Fatalf("cfg = %+v", cfg)
	}
	if !cfg.SessionCookieSecure {
		t.Fatal("expected cookie secure")
	}
	if !cfg.AllowsDomain("mail.example.com") {
		t.Fatal("example.com should match")
	}
	if !cfg.AllowsDomain("sub.test") || !cfg.AllowsDomain("test") {
		t.Fatal("*.test should match zone and subdomains")
	}
	if cfg.AllowsDomain("other.net") {
		t.Fatal("other.net should not match")
	}
}

func TestInvalidValues(t *testing.T) {
	cases := []map[string]string{
		{"MAILVOID_SMTP_PORTS": "nope"},
		{"MAILVOID_SMTP_PORTS": "0"},
		{"MAILVOID_SMTP_PORTS": "70000"},
		{"MAILVOID_SWEEP_INTERVAL": "soon"},
		{"MAILVOID_MAX_MESSAGE_BYTES": "big"},
		{"MAILVOID_ADMIN_RESET_PASSWORD": "maybe"},
	}
	for _, env := range cases {
		if _, err := config.Load(nil, func(k string) string { return env[k] }); err == nil {
			t.Errorf("expected error for %v", env)
		}
	}
}

func TestCustomPaths(t *testing.T) {
	env := map[string]string{
		"MAILVOID_DATA_DIR":  "/tmp/mv",
		"MAILVOID_DB_PATH":   "/tmp/mv/custom.db",
		"MAILVOID_EVENT_LOG": "/tmp/mv/custom.jsonl",
	}
	cfg, err := config.Load(nil, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBPath != "/tmp/mv/custom.db" || cfg.EventLogPath != "/tmp/mv/custom.jsonl" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestValidateErrors(t *testing.T) {
	cases := []config.Config{
		func() config.Config { c := config.Default(); c.SMTPPorts = nil; return c }(),
		func() config.Config { c := config.Default(); c.SMTPPorts = []int{0}; return c }(),
		func() config.Config { c := config.Default(); c.SMTPPorts = []int{70000}; return c }(),
		func() config.Config { c := config.Default(); c.HTTPAddr = ""; return c }(),
		func() config.Config { c := config.Default(); c.AdminUser = ""; return c }(),
	}
	for _, cfg := range cases {
		if err := cfg.Validate(); err == nil {
			t.Errorf("expected validation error for %+v", cfg)
		}
	}
	if err := config.Default().Validate(); err != nil {
		t.Fatalf("default should validate: %v", err)
	}
}

func TestFinalizeDefaults(t *testing.T) {
	env := map[string]string{
		"MAILVOID_SWEEP_INTERVAL":    "-1s",
		"MAILVOID_MAX_MESSAGE_BYTES": "-5",
		"MAILVOID_READ_TIMEOUT":      "-1s",
		"MAILVOID_WEBDAV_PASSWORD":   "dav-secret",
	}
	cfg, err := config.Load([]string{"-data-dir", "", "-webdav-user", ""}, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != "./data" || cfg.DBPath != "data/mailvoid.db" || cfg.EventLogPath != "data/events.jsonl" {
		t.Fatalf("paths = %+v", cfg)
	}
	if cfg.SweepInterval != 30*time.Second || cfg.MaxMessageBytes != 25<<20 || cfg.ReadTimeout != 60*time.Second {
		t.Fatalf("defaults = %+v", cfg)
	}
	if cfg.WebDAVUser != "mailvoid" || cfg.WebDAVPassword != "dav-secret" {
		t.Fatalf("webdav = %q/%q", cfg.WebDAVUser, cfg.WebDAVPassword)
	}
}

func TestMoreInvalidEnvironment(t *testing.T) {
	for _, env := range []map[string]string{
		{"MAILVOID_READ_TIMEOUT": "later"},
		{"MAILVOID_COOKIE_SECURE": "maybe"},
		{"MAILVOID_DB_PATH": ""},
	} {
		if _, err := config.Load(nil, func(k string) string { return env[k] }); err != nil && env["MAILVOID_DB_PATH"] != "" {
			t.Logf("unexpected error for %v: %v", env, err)
		}
	}
	if _, err := config.Load(nil, func(k string) string { return map[string]string{"MAILVOID_READ_TIMEOUT": "later"}[k] }); err == nil {
		t.Fatal("expected read-timeout error")
	}
	if _, err := config.Load(nil, func(k string) string { return map[string]string{"MAILVOID_COOKIE_SECURE": "maybe"}[k] }); err == nil {
		t.Fatal("expected cookie-secure error")
	}
}

func TestFlagsAndSecretOptions(t *testing.T) {
	cfg, err := config.Load([]string{
		"-api-token", "token-123",
		"-webdav-user", "dav",
		"-webdav-password", "dav-secret",
		"-domains", "Example.Test, *.example.test,Example.Test",
		"-smtp-ports", "1025 2525,1025",
		"-search-enabled=false",
		"-search-index", "/tmp/search-index",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIToken != "token-123" || cfg.WebDAVUser != "dav" || cfg.WebDAVPassword != "dav-secret" {
		t.Fatalf("secrets = %+v", cfg)
	}
	if len(cfg.Domains) != 2 || len(cfg.SMTPPorts) != 2 {
		t.Fatalf("lists = %+v %+v", cfg.Domains, cfg.SMTPPorts)
	}
	if cfg.SearchEnabled || cfg.SearchIndexPath != "/tmp/search-index" {
		t.Fatalf("search flags = %v %q", cfg.SearchEnabled, cfg.SearchIndexPath)
	}

	for _, args := range [][]string{{"-smtp-ports", "nope"}, {"-unknown"}} {
		if _, err := config.Load(args, nil); err == nil {
			t.Errorf("expected error for %v", args)
		}
	}
}

func TestAllowsDomainEdges(t *testing.T) {
	cfg := config.Default()
	cfg.Domains = []string{"Example.Test.", "*.zone.test", "*.wild.test"}
	cases := map[string]bool{
		"example.test":   true,
		"EXAMPLE.TEST.":  true,
		"zone.test":      true,
		"mail.zone.test": true,
		"wild.test":      true,
		"a.b.wild.test":  true,
		"other.test":     false,
		"":               false,
	}
	for domain, want := range cases {
		if got := cfg.AllowsDomain(domain); got != want {
			t.Errorf("AllowsDomain(%q) = %v, want %v", domain, got, want)
		}
	}
	cfg.Domains = nil
	if !cfg.AllowsDomain("anything.test") {
		t.Fatal("empty domains should accept all")
	}
	cfg.Domains = []string{"*"}
	if !cfg.AllowsDomain("anything.test") {
		t.Fatal("wildcard should accept all")
	}
}
