// Package config loads MailVoid settings from defaults, environment variables,
// and command-line flags.
package config

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config is the fully resolved runtime configuration.
type Config struct {
	SMTPHost            string
	SMTPPorts           []int
	HTTPAddr            string
	Domains             []string
	DataDir             string
	DBPath              string
	EventLogPath        string
	AdminUser           string
	AdminPassword       string
	AdminResetPassword  bool
	APIToken            string
	WebDAVUser          string
	WebDAVPassword      string
	SearchEnabled       bool
	SearchIndexPath     string
	SweepInterval       time.Duration
	MaxMessageBytes     int64
	ReadTimeout         time.Duration
	SessionCookieSecure bool
}

// Default returns the baseline configuration.
func Default() Config {
	return Config{
		SMTPHost:        "0.0.0.0",
		SMTPPorts:       []int{1025, 2525},
		HTTPAddr:        ":8025",
		Domains:         []string{"*"},
		DataDir:         "./data",
		AdminUser:       "admin",
		WebDAVUser:      "mailvoid",
		SearchEnabled:   true,
		SweepInterval:   30 * time.Second,
		MaxMessageBytes: 25 << 20,
		ReadTimeout:     60 * time.Second,
	}
}

// Load merges defaults, environment variables, and flags. Flags win over
// environment values. getenv may be nil.
func Load(args []string, getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	cfg := Default()
	if err := applyEnv(&cfg, getenv); err != nil {
		return Config{}, err
	}
	if err := applyFlags(&cfg, args); err != nil {
		return Config{}, err
	}
	cfg.finalize()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) finalize() {
	if c.DataDir == "" {
		c.DataDir = "./data"
	}
	if c.DBPath == "" {
		c.DBPath = filepath.Join(c.DataDir, "mailvoid.db")
	}
	if c.SearchIndexPath == "" {
		c.SearchIndexPath = filepath.Join(c.DataDir, "search.bleve")
	}
	if c.EventLogPath == "" {
		c.EventLogPath = filepath.Join(c.DataDir, "events.jsonl")
	}
	if c.SMTPHost == "" {
		c.SMTPHost = "0.0.0.0"
	}
	if c.HTTPAddr == "" {
		c.HTTPAddr = ":8025"
	}
	if c.WebDAVPassword != "" && c.WebDAVUser == "" {
		c.WebDAVUser = "mailvoid"
	}
	if c.SweepInterval <= 0 {
		c.SweepInterval = 30 * time.Second
	}
	if c.MaxMessageBytes <= 0 {
		c.MaxMessageBytes = 25 << 20
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = 60 * time.Second
	}
}

// Validate checks that the resolved configuration is usable.
func (c Config) Validate() error {
	if len(c.SMTPPorts) == 0 {
		return fmt.Errorf("at least one SMTP port is required")
	}
	for _, port := range c.SMTPPorts {
		if port < 1 || port > 65535 {
			return fmt.Errorf("SMTP port %d is out of range", port)
		}
	}
	if c.HTTPAddr == "" {
		return fmt.Errorf("HTTP address is required")
	}
	if c.AdminUser == "" {
		return fmt.Errorf("admin username is required")
	}
	return nil
}

// AllowsDomain reports whether the domain is accepted. An empty list or "*"
// accepts every domain; "*.example.com" accepts the zone and its subdomains.
func (c Config) AllowsDomain(domain string) bool {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if domain == "" {
		return false
	}
	if len(c.Domains) == 0 {
		return true
	}
	for _, allowed := range c.Domains {
		allowed = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(allowed), "."))
		switch {
		case allowed == "*":
			return true
		case strings.HasPrefix(allowed, "*."):
			zone := allowed[2:]
			if domain == zone || strings.HasSuffix(domain, "."+zone) {
				return true
			}
		case allowed == domain:
			return true
		}
	}
	return false
}

func applyEnv(c *Config, getenv func(string) string) error {
	if v := getenv("MAILVOID_SMTP_HOST"); v != "" {
		c.SMTPHost = v
	}
	if v := getenv("MAILVOID_SMTP_PORTS"); v != "" {
		ports, err := parsePorts(v)
		if err != nil {
			return fmt.Errorf("MAILVOID_SMTP_PORTS: %w", err)
		}
		c.SMTPPorts = ports
	}
	if v := getenv("MAILVOID_HTTP_ADDR"); v != "" {
		c.HTTPAddr = v
	}
	if v := getenv("MAILVOID_DOMAINS"); v != "" {
		c.Domains = parseList(v)
	}
	if v := getenv("MAILVOID_DATA_DIR"); v != "" {
		c.DataDir = v
	}
	if v := getenv("MAILVOID_DB_PATH"); v != "" {
		c.DBPath = v
	}
	if v := getenv("MAILVOID_EVENT_LOG"); v != "" {
		c.EventLogPath = v
	}
	if v := getenv("MAILVOID_ADMIN_USER"); v != "" {
		c.AdminUser = v
	}
	if v := getenv("MAILVOID_ADMIN_PASSWORD"); v != "" {
		c.AdminPassword = v
	}
	if v := getenv("MAILVOID_API_TOKEN"); v != "" {
		c.APIToken = v
	}
	if v := getenv("MAILVOID_WEBDAV_USER"); v != "" {
		c.WebDAVUser = v
	}
	if v := getenv("MAILVOID_WEBDAV_PASSWORD"); v != "" {
		c.WebDAVPassword = v
	}
	if v := getenv("MAILVOID_SEARCH_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("MAILVOID_SEARCH_ENABLED: %w", err)
		}
		c.SearchEnabled = b
	}
	if v := getenv("MAILVOID_SEARCH_INDEX"); v != "" {
		c.SearchIndexPath = v
	}
	if v := getenv("MAILVOID_ADMIN_RESET_PASSWORD"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("MAILVOID_ADMIN_RESET_PASSWORD: %w", err)
		}
		c.AdminResetPassword = b
	}
	if v := getenv("MAILVOID_SWEEP_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("MAILVOID_SWEEP_INTERVAL: %w", err)
		}
		c.SweepInterval = d
	}
	if v := getenv("MAILVOID_MAX_MESSAGE_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("MAILVOID_MAX_MESSAGE_BYTES: %w", err)
		}
		c.MaxMessageBytes = n
	}
	if v := getenv("MAILVOID_READ_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("MAILVOID_READ_TIMEOUT: %w", err)
		}
		c.ReadTimeout = d
	}
	if v := getenv("MAILVOID_COOKIE_SECURE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("MAILVOID_COOKIE_SECURE: %w", err)
		}
		c.SessionCookieSecure = b
	}
	return nil
}

func applyFlags(c *Config, args []string) error {
	fs := flag.NewFlagSet("mailvoid", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	smtpHost := fs.String("smtp-host", c.SMTPHost, "SMTP listen host")
	smtpPorts := fs.String("smtp-ports", joinInts(c.SMTPPorts), "comma-separated SMTP ports")
	httpAddr := fs.String("http-addr", c.HTTPAddr, "HTTP listen address")
	domains := fs.String("domains", strings.Join(c.Domains, ","), "comma-separated accepted domains")
	dataDir := fs.String("data-dir", c.DataDir, "data directory")
	dbPath := fs.String("db-path", c.DBPath, "Genji database path")
	eventLog := fs.String("event-log", c.EventLogPath, "JSONL event log path")
	adminUser := fs.String("admin-user", c.AdminUser, "bootstrap administrator username")
	adminPassword := fs.String("admin-password", c.AdminPassword, "bootstrap administrator password")
	adminReset := fs.Bool("admin-reset-password", c.AdminResetPassword, "reset the bootstrap administrator password")
	apiToken := fs.String("api-token", c.APIToken, "bearer token for /api/v1 and /mcp")
	webdavUser := fs.String("webdav-user", c.WebDAVUser, "WebDAV basic-auth username")
	webdavPassword := fs.String("webdav-password", c.WebDAVPassword, "WebDAV basic-auth password; empty disables WebDAV")
	searchEnabled := fs.Bool("search-enabled", c.SearchEnabled, "enable the Bleve full-text search index")
	searchIndex := fs.String("search-index", c.SearchIndexPath, "Bleve search index directory")
	sweep := fs.Duration("sweep-interval", c.SweepInterval, "expired-message sweep interval")
	maxBytes := fs.Int64("max-message-bytes", c.MaxMessageBytes, "maximum accepted message size")
	readTimeout := fs.Duration("read-timeout", c.ReadTimeout, "SMTP read timeout")
	cookieSecure := fs.Bool("cookie-secure", c.SessionCookieSecure, "mark session cookies Secure")

	if err := fs.Parse(args); err != nil {
		return err
	}

	c.SMTPHost = *smtpHost
	ports, err := parsePorts(*smtpPorts)
	if err != nil {
		return fmt.Errorf("smtp-ports: %w", err)
	}
	c.SMTPPorts = ports
	c.HTTPAddr = *httpAddr
	c.Domains = parseList(*domains)
	c.DataDir = *dataDir
	c.DBPath = *dbPath
	c.EventLogPath = *eventLog
	c.AdminUser = *adminUser
	c.AdminPassword = *adminPassword
	c.AdminResetPassword = *adminReset
	c.APIToken = *apiToken
	c.WebDAVUser = *webdavUser
	c.WebDAVPassword = *webdavPassword
	c.SearchEnabled = *searchEnabled
	c.SearchIndexPath = *searchIndex
	c.SweepInterval = *sweep
	c.MaxMessageBytes = *maxBytes
	c.ReadTimeout = *readTimeout
	c.SessionCookieSecure = *cookieSecure
	return nil
}

func parsePorts(value string) ([]int, error) {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	if len(parts) == 0 {
		return nil, fmt.Errorf("no ports provided")
	}
	seen := make(map[int]bool, len(parts))
	ports := make([]int, 0, len(parts))
	for _, part := range parts {
		port, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("invalid port %q", part)
		}
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("port %d out of range", port)
		}
		if seen[port] {
			continue
		}
		seen[port] = true
		ports = append(ports, port)
	}
	return ports, nil
}

func parseList(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	seen := make(map[string]bool, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}

func joinInts(values []int) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ",")
}
