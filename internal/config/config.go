// Package config loads MailVoid settings from defaults, an optional TOML file,
// environment variables, and command-line flags.
package config

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SMTPService describes one SMTP listener bound to a single interface and port.
// Name and Aliases give the listener a stable identity so it can keep its own
// traditional (non-event) log file.
type SMTPService struct {
	Name    string
	Host    string
	Port    int
	Aliases []string
	LogPath string
}

// Address returns the host:port listen address.
func (s SMTPService) Address() string {
	host := strings.TrimSpace(s.Host)
	if host == "" {
		host = "0.0.0.0"
	}
	return net.JoinHostPort(host, strconv.Itoa(s.Port))
}

// LogName returns the safe base name used for the service log file.
func (s SMTPService) LogName() string {
	name := strings.TrimSpace(s.Name)
	if name == "" {
		for _, alias := range s.Aliases {
			if alias = strings.TrimSpace(alias); alias != "" {
				name = alias
				break
			}
		}
	}
	if name == "" {
		name = AutoSMTPServiceName(s.Host, s.Port)
	}
	return sanitizeFileName(name)
}

// AutoSMTPServiceName builds a stable name for a listener without an explicit
// name.
func AutoSMTPServiceName(host string, port int) string {
	host = strings.TrimSpace(host)
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return fmt.Sprintf("smtp-%d", port)
	}
	return fmt.Sprintf("smtp-%s-%d", sanitizeFileName(host), port)
}

func sanitizeFileName(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		return "smtp"
	}
	return out
}

// Config is the fully resolved runtime configuration.
type Config struct {
	Hostname            string
	Scheme              string
	PublicURL           string
	SMTPHost            string
	SMTPPorts           []int
	SMTPServices        []SMTPService
	HTTPAddr            string
	Domains             []string
	DataDir             string
	DBPath              string
	EventLogDisabled    bool
	EventLogPath        string
	LogEnabled          bool
	LogDir              string
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
		Hostname:        "localhost",
		Scheme:          "http",
		SMTPHost:        "0.0.0.0",
		SMTPPorts:       []int{1025, 2525},
		HTTPAddr:        ":8025",
		Domains:         []string{"*"},
		DataDir:         "./data",
		AdminUser:       "admin",
		WebDAVUser:      "mailvoid",
		SearchEnabled:   true,
		LogEnabled:      true,
		SweepInterval:   30 * time.Second,
		MaxMessageBytes: 25 << 20,
		ReadTimeout:     60 * time.Second,
	}
}

// Load merges defaults, the TOML configuration file, environment variables, and
// flags. Later sources win over earlier ones. getenv may be nil.
func Load(args []string, getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	cfg := Default()

	path, explicit, err := configPath(args, getenv)
	if err != nil {
		return Config{}, err
	}
	if path != "" {
		if err := applyFile(&cfg, path, explicit); err != nil {
			return Config{}, err
		}
	}
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

func configPath(args []string, getenv func(string) string) (string, bool, error) {
	if v := strings.TrimSpace(getenv("MAILVOID_CONFIG")); v != "" {
		return v, true, nil
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		switch {
		case arg == "-config" || arg == "--config":
			if i+1 >= len(args) {
				return "", false, fmt.Errorf("%s requires a path", arg)
			}
			return strings.TrimSpace(args[i+1]), true, nil
		case strings.HasPrefix(arg, "-config="):
			return strings.TrimSpace(strings.TrimPrefix(arg, "-config=")), true, nil
		case strings.HasPrefix(arg, "--config="):
			return strings.TrimSpace(strings.TrimPrefix(arg, "--config=")), true, nil
		}
	}
	return "mailvoid.toml", false, nil
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
	if c.LogDir == "" {
		c.LogDir = filepath.Join(c.DataDir, "logs")
	}
	if c.SMTPHost == "" {
		c.SMTPHost = "0.0.0.0"
	}
	if c.HTTPAddr == "" {
		c.HTTPAddr = ":8025"
	}
	if c.Hostname == "" {
		c.Hostname = "localhost"
	}
	if c.Scheme == "" {
		c.Scheme = "http"
	}
	c.Scheme = strings.ToLower(strings.TrimSpace(c.Scheme))
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
	if len(c.SMTPServices) == 0 {
		c.SMTPServices = servicesFromHostPorts(c.SMTPHost, c.SMTPPorts)
	}
	for i := range c.SMTPServices {
		if strings.TrimSpace(c.SMTPServices[i].Host) == "" {
			c.SMTPServices[i].Host = "0.0.0.0"
		}
	}
}

// Validate checks that the resolved configuration is usable.
func (c Config) Validate() error {
	services := c.EffectiveSMTPServices()
	if len(services) == 0 {
		return fmt.Errorf("at least one SMTP service is required")
	}
	seen := map[string]string{}
	for _, svc := range services {
		if svc.Port < 1 || svc.Port > 65535 {
			return fmt.Errorf("SMTP port %d is out of range", svc.Port)
		}
		name := strings.ToLower(svc.LogName())
		if other, ok := seen[name]; ok {
			return fmt.Errorf("SMTP service name %q is already used by %s", svc.LogName(), other)
		}
		seen[name] = svc.Address()
		for _, alias := range svc.Aliases {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if alias == "" {
				continue
			}
			if other, ok := seen[alias]; ok {
				return fmt.Errorf("SMTP service alias %q is already used by %s", alias, other)
			}
			seen[alias] = svc.Address()
		}
	}
	if c.HTTPAddr == "" {
		return fmt.Errorf("HTTP address is required")
	}
	if c.AdminUser == "" {
		return fmt.Errorf("admin username is required")
	}
	switch c.Scheme {
	case "", "http", "https":
	default:
		return fmt.Errorf("scheme must be http or https, got %q", c.Scheme)
	}
	if c.PublicURL != "" {
		parsed, err := url.Parse(c.PublicURL)
		if err != nil || parsed.Host == "" {
			return fmt.Errorf("public_url must be an absolute URL, got %q", c.PublicURL)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("public_url must use http or https, got %q", parsed.Scheme)
		}
	}
	return nil
}

// EffectiveSMTPServices returns the configured services, generating one per
// host/port pair when the config only defines SMTPHost and SMTPPorts.
func (c Config) EffectiveSMTPServices() []SMTPService {
	if len(c.SMTPServices) > 0 {
		out := make([]SMTPService, len(c.SMTPServices))
		copy(out, c.SMTPServices)
		for i := range out {
			out[i].Aliases = append([]string(nil), out[i].Aliases...)
		}
		return out
	}
	return servicesFromHostPorts(c.SMTPHost, c.SMTPPorts)
}

// SMTPServicePorts returns the unique ports used by the configured services.
func (c Config) SMTPServicePorts() []int {
	seen := make(map[int]bool)
	var out []int
	for _, svc := range c.EffectiveSMTPServices() {
		if seen[svc.Port] {
			continue
		}
		seen[svc.Port] = true
		out = append(out, svc.Port)
	}
	return out
}

// SMTPAddresses returns the unique host:port listener addresses.
func (c Config) SMTPAddresses() []string {
	seen := make(map[string]bool)
	var out []string
	for _, svc := range c.EffectiveSMTPServices() {
		addr := svc.Address()
		if seen[addr] {
			continue
		}
		seen[addr] = true
		out = append(out, addr)
	}
	return out
}

// EventLogEnabled reports whether the append-only JSONL event log is enabled.
func (c Config) EventLogEnabled() bool { return !c.EventLogDisabled }

// WebPort returns the port portion of HTTPAddr, or zero when it cannot be
// parsed.
func (c Config) WebPort() int {
	_, portStr, err := net.SplitHostPort(c.HTTPAddr)
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}
	return port
}

// WebBind returns the interface portion of HTTPAddr.
func (c Config) WebBind() string {
	host, _, err := net.SplitHostPort(c.HTTPAddr)
	if err != nil {
		return c.HTTPAddr
	}
	return host
}

// PublicBaseURL returns the absolute prefix used for generated links.
func (c Config) PublicBaseURL() string {
	if v := strings.TrimSpace(c.PublicURL); v != "" {
		return strings.TrimRight(v, "/")
	}
	host := strings.TrimSpace(c.Hostname)
	if host == "" {
		return ""
	}
	if strings.Contains(host, "://") {
		return strings.TrimRight(host, "/")
	}
	scheme := strings.ToLower(strings.TrimSpace(c.Scheme))
	if scheme == "" {
		scheme = "http"
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		port := c.WebPort()
		if port > 0 && !isDefaultWebPort(scheme, port) {
			host = net.JoinHostPort(host, strconv.Itoa(port))
		}
	}
	return scheme + "://" + host
}

func isDefaultWebPort(scheme string, port int) bool {
	return (scheme == "http" && port == 80) || (scheme == "https" && port == 443)
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

func servicesFromHostPorts(host string, ports []int) []SMTPService {
	if host == "" {
		host = "0.0.0.0"
	}
	out := make([]SMTPService, 0, len(ports))
	for _, port := range ports {
		out = append(out, SMTPService{
			Host: host,
			Port: port,
			Name: AutoSMTPServiceName(host, port),
		})
	}
	return out
}

func servicesFromHostsPorts(hosts []string, ports []int) []SMTPService {
	var out []SMTPService
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			host = "0.0.0.0"
		}
		for _, port := range ports {
			out = append(out, SMTPService{
				Host: host,
				Port: port,
				Name: AutoSMTPServiceName(host, port),
			})
		}
	}
	return out
}

func applyEnv(c *Config, getenv func(string) string) error {
	if v := getenv("MAILVOID_HOSTNAME"); v != "" {
		c.Hostname = v
	}
	if v := getenv("MAILVOID_SCHEME"); v != "" {
		c.Scheme = v
	}
	if v := getenv("MAILVOID_PUBLIC_URL"); v != "" {
		c.PublicURL = v
	}
	if v := getenv("MAILVOID_HTTP_ADDR"); v != "" {
		c.HTTPAddr = v
	}
	webBind := getenv("MAILVOID_WEB_BIND")
	webPort := getenv("MAILVOID_WEB_PORT")
	if webBind != "" || webPort != "" {
		if err := applyWebAddr(c, webBind, webPort); err != nil {
			return fmt.Errorf("MAILVOID_WEB_PORT: %w", err)
		}
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
	if v := getenv("MAILVOID_EVENT_LOG_ENABLED"); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("MAILVOID_EVENT_LOG_ENABLED: %w", err)
		}
		c.EventLogDisabled = !enabled
	}
	if v := getenv("MAILVOID_LOG_ENABLED"); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("MAILVOID_LOG_ENABLED: %w", err)
		}
		c.LogEnabled = enabled
	}
	if v := getenv("MAILVOID_LOG_DIR"); v != "" {
		c.LogDir = v
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

	portsChanged := false
	if v := getenv("MAILVOID_SMTP_PORTS"); v != "" {
		ports, err := parsePorts(v)
		if err != nil {
			return fmt.Errorf("MAILVOID_SMTP_PORTS: %w", err)
		}
		c.SMTPPorts = ports
		portsChanged = true
	}
	if v := getenv("MAILVOID_SMTP_HOSTS"); v != "" {
		c.SMTPServices = servicesFromHostsPorts(parseList(v), c.SMTPPorts)
	} else if v := getenv("MAILVOID_SMTP_HOST"); v != "" {
		c.SMTPHost = v
		c.SMTPServices = nil
	} else if portsChanged {
		c.SMTPServices = nil
	}
	return nil
}

func applyFlags(c *Config, args []string) error {
	fs := flag.NewFlagSet("mailvoid", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	_ = fs.String("config", "", "path to mailvoid.toml")
	hostname := fs.String("hostname", c.Hostname, "public hostname used for absolute links")
	scheme := fs.String("scheme", c.Scheme, "public URL scheme (http or https)")
	publicURL := fs.String("public-url", c.PublicURL, "absolute public base URL override")
	smtpHost := fs.String("smtp-host", c.SMTPHost, "SMTP listen host")
	smtpPorts := fs.String("smtp-ports", joinInts(c.SMTPPorts), "comma-separated SMTP ports")
	smtpHosts := fs.String("smtp-hosts", "", "comma-separated SMTP listen hosts")
	httpAddr := fs.String("http-addr", c.HTTPAddr, "HTTP listen address")
	webBind := fs.String("web-bind", "", "HTTP listen interface")
	webPort := fs.Int("web-port", 0, "HTTP listen port")
	domains := fs.String("domains", strings.Join(c.Domains, ","), "comma-separated accepted domains")
	dataDir := fs.String("data-dir", c.DataDir, "data directory")
	dbPath := fs.String("db-path", c.DBPath, "Genji database path")
	eventLog := fs.String("event-log", c.EventLogPath, "JSONL event log path")
	eventLogEnabled := fs.Bool("event-log-enabled", c.EventLogEnabled(), "enable the JSONL event log")
	logEnabled := fs.Bool("log-enabled", c.LogEnabled, "enable per-service traditional logs")
	logDir := fs.String("log-dir", c.LogDir, "directory for per-service logs")
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
	set := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	if set["hostname"] {
		c.Hostname = *hostname
	}
	if set["scheme"] {
		c.Scheme = *scheme
	}
	if set["public-url"] {
		c.PublicURL = *publicURL
	}
	if set["smtp-ports"] {
		ports, err := parsePorts(*smtpPorts)
		if err != nil {
			return fmt.Errorf("smtp-ports: %w", err)
		}
		c.SMTPPorts = ports
		if !set["smtp-host"] && !set["smtp-hosts"] {
			c.SMTPServices = nil
		}
	}
	if set["smtp-hosts"] {
		c.SMTPServices = servicesFromHostsPorts(parseList(*smtpHosts), c.SMTPPorts)
	}
	if set["smtp-host"] {
		c.SMTPHost = *smtpHost
		c.SMTPServices = nil
	}
	if set["http-addr"] {
		c.HTTPAddr = *httpAddr
	}
	if set["web-bind"] || set["web-port"] {
		host, port := splitHTTPAddr(c.HTTPAddr)
		if set["web-bind"] {
			host = *webBind
		}
		if set["web-port"] {
			port = *webPort
		}
		c.HTTPAddr = joinHTTPAddr(host, port)
	}
	c.Domains = parseList(*domains)
	c.DataDir = *dataDir
	c.DBPath = *dbPath
	c.EventLogPath = *eventLog
	c.EventLogDisabled = !*eventLogEnabled
	c.LogEnabled = *logEnabled
	c.LogDir = *logDir
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

func applyWebAddr(c *Config, bind, port string) error {
	host, currentPort := splitHTTPAddr(c.HTTPAddr)
	if bind != "" {
		host = bind
	}
	if port != "" {
		p, err := strconv.Atoi(port)
		if err != nil || p < 1 || p > 65535 {
			return fmt.Errorf("invalid port %q", port)
		}
		currentPort = p
	}
	c.HTTPAddr = joinHTTPAddr(host, currentPort)
	return nil
}

func splitHTTPAddr(addr string) (string, int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, 0
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return host, 0
	}
	return host, port
}

func joinHTTPAddr(host string, port int) string {
	if port <= 0 {
		if host == "" {
			return ":8025"
		}
		return host
	}
	if host == "" {
		return fmt.Sprintf(":%d", port)
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
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
