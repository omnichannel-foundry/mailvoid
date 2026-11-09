package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/smtp"
	"net/textproto"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options controls server startup and test execution.
type Options struct {
	ServerPath    string
	SuiteDir      string
	BaseURL       string
	SMTPAddr      string
	AdminUser     string
	AdminPassword string
	Keep          bool
	Verbose       bool
}

// Server is a launched MailVoid process or an external endpoint.
type Server struct {
	BaseURL   string
	SMTPHost  string
	SMTPPorts []int
	DataDir   string

	keep   bool
	cmd    *exec.Cmd
	logs   *syncBuffer
	closed bool
	mu     sync.Mutex
}

// StepResult records one executed step.
type StepResult struct {
	Name     string
	Type     string
	Logs     []string
	Err      error
	Duration time.Duration
}

// TestResult records one test.
type TestResult struct {
	File        string
	Name        string
	Description string
	Passed      bool
	Duration    time.Duration
	Steps       []StepResult
	Err         error
}

func (r TestResult) Logs() []string {
	var lines []string
	for _, step := range r.Steps {
		lines = append(lines, step.Logs...)
	}
	return lines
}

// SyncBuffer is a concurrency-safe log buffer.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// StartServer launches the MailVoid binary on free local ports, or returns an
// external endpoint when BaseURL is set.
func StartServer(opts Options) (*Server, error) {
	if opts.BaseURL != "" {
		host, port, err := net.SplitHostPort(opts.SMTPAddr)
		if err != nil {
			return nil, fmt.Errorf("-smtp-addr must be host:port: %w", err)
		}
		parsedPort, err := strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("invalid smtp port: %w", err)
		}
		return &Server{
			BaseURL: strings.TrimRight(opts.BaseURL, "/"), SMTPHost: host,
			SMTPPorts: []int{parsedPort},
		}, nil
	}

	serverPath := opts.ServerPath
	if serverPath == "" {
		serverPath = filepath.Join("bin", "mailvoid")
	}
	if _, err := os.Stat(serverPath); err != nil {
		return nil, fmt.Errorf("server binary %s not found (run make build): %w", serverPath, err)
	}

	httpPort, err := freePort()
	if err != nil {
		return nil, err
	}
	smtpPort1, err := freePort()
	if err != nil {
		return nil, err
	}
	smtpPort2, err := freePort()
	if err != nil {
		return nil, err
	}
	dataDir, err := os.MkdirTemp("", "mailvoid-it-*")
	if err != nil {
		return nil, err
	}

	env := filteredEnvironment()
	env = append(env,
		"MAILVOID_HTTP_ADDR=127.0.0.1:"+strconv.Itoa(httpPort),
		"MAILVOID_SMTP_HOST=127.0.0.1",
		"MAILVOID_SMTP_PORTS="+strconv.Itoa(smtpPort1)+","+strconv.Itoa(smtpPort2),
		"MAILVOID_DATA_DIR="+dataDir,
		"MAILVOID_DB_PATH="+filepath.Join(dataDir, "mailvoid.db"),
		"MAILVOID_EVENT_LOG="+filepath.Join(dataDir, "events.jsonl"),
		"MAILVOID_EVENT_LOG_ENABLED=true",
		"MAILVOID_LOG_DIR="+filepath.Join(dataDir, "logs"),
		"MAILVOID_SEARCH_INDEX="+filepath.Join(dataDir, "search.bleve"),
		"MAILVOID_HOSTNAME=127.0.0.1",
		"MAILVOID_SCHEME=http",
		"MAILVOID_PUBLIC_URL=",
		"MAILVOID_ADMIN_USER="+opts.AdminUser,
		"MAILVOID_ADMIN_PASSWORD="+opts.AdminPassword,
		"MAILVOID_DOMAINS=*",
		"MAILVOID_SEARCH_ENABLED=true",
		"MAILVOID_WEBDAV_USER=",
		"MAILVOID_WEBDAV_PASSWORD=",
		"MAILVOID_API_TOKEN=",
		"MAILVOID_SWEEP_INTERVAL=1s",
	)

	logs := &syncBuffer{}
	cmd := exec.Command(serverPath)
	cmd.Env = env
	cmd.Stdout = logs
	cmd.Stderr = logs
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dataDir)
		return nil, fmt.Errorf("start server: %w", err)
	}

	server := &Server{
		BaseURL:   "http://127.0.0.1:" + strconv.Itoa(httpPort),
		SMTPHost:  "127.0.0.1",
		SMTPPorts: []int{smtpPort1, smtpPort2},
		DataDir:   dataDir,
		keep:      opts.Keep,
		cmd:       cmd,
		logs:      logs,
	}
	if err := server.waitReady(20 * time.Second); err != nil {
		_ = server.Stop()
		os.RemoveAll(dataDir)
		return nil, fmt.Errorf("%w\nserver log:\n%s", err, logs.String())
	}
	return server, nil
}

func (s *Server) waitReady(timeout time.Duration) error {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(s.BaseURL + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("server did not become ready")
}

// Stop terminates a launched server. External servers are left alone.
func (s *Server) Stop() error {
	if s == nil || s.cmd == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	_ = s.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		_ = s.cmd.Process.Kill()
		return <-done
	}
}

// Logs returns captured server output.
func (s *Server) Logs() string {
	if s == nil || s.logs == nil {
		return ""
	}
	return s.logs.String()
}

func (s *Server) Cleanup() {
	if s != nil && s.cmd != nil && !s.KeepData() {
		_ = os.RemoveAll(s.DataDir)
	}
}

func (s *Server) KeepData() bool { return s != nil && s.keep }

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func filteredEnvironment() []string {
	var out []string
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "MAILVOID_") {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// Harness executes one test with an isolated session and MCP clients.
type Harness struct {
	baseURL   string
	smtpHost  string
	smtpPorts []int
	adminUser string
	adminPass string

	client      *http.Client
	jar         http.CookieJar
	mcpMu       sync.Mutex
	mcpSessions map[string]*mcp.ClientSession
	vars        map[string]string
	runID       string
}

// NewHarness logs in as the administrator and builds the variable map.
func NewHarness(server *Server, opts Options) (*Harness, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	h := &Harness{
		baseURL:     strings.TrimRight(server.BaseURL, "/"),
		smtpHost:    server.SMTPHost,
		smtpPorts:   server.SMTPPorts,
		adminUser:   opts.AdminUser,
		adminPass:   opts.AdminPassword,
		client:      client,
		jar:         jar,
		mcpSessions: make(map[string]*mcp.ClientSession),
		vars:        make(map[string]string),
		runID:       randomRunID(),
	}
	form := url.Values{"username": {opts.AdminUser}, "password": {opts.AdminPassword}}
	resp, err := client.PostForm(h.baseURL+"/login", form)
	if err != nil {
		return nil, fmt.Errorf("admin login: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		return nil, fmt.Errorf("admin login status %d", resp.StatusCode)
	}

	h.vars["run_id"] = h.runID
	h.vars["base_url"] = h.baseURL
	h.vars["smtp_host"] = h.smtpHost
	h.vars["smtp_port"] = strconv.Itoa(h.smtpPorts[0])
	if len(h.smtpPorts) > 1 {
		h.vars["smtp_port2"] = strconv.Itoa(h.smtpPorts[1])
	} else {
		h.vars["smtp_port2"] = h.vars["smtp_port"]
	}
	h.vars["admin_user"] = opts.AdminUser
	h.vars["admin_password"] = opts.AdminPassword
	h.vars["domain"] = "example.test"
	return h, nil
}

func (h *Harness) Close() {
	h.mcpMu.Lock()
	defer h.mcpMu.Unlock()
	for _, session := range h.mcpSessions {
		_ = session.Close()
	}
	h.mcpSessions = make(map[string]*mcp.ClientSession)
}

func randomRunID() string {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}

// RunTest executes one JTD test definition.
func (h *Harness) RunTest(ctx context.Context, test SuiteTest) TestResult {
	started := time.Now()
	result := TestResult{File: test.File, Name: test.Definition.Name, Description: test.Definition.Description, Passed: true}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	for _, originalStep := range test.Definition.Steps {
		step, err := interpolateStep(originalStep, h.vars)
		stepResult := StepResult{Name: step.Name, Type: step.Type}
		if err != nil {
			stepResult.Err = err
			result.Steps = append(result.Steps, stepResult)
			result.Passed = false
			result.Err = err
			break
		}
		stepStarted := time.Now()
		logs, err := h.executeStep(ctx, step)
		stepResult.Logs = logs
		stepResult.Err = err
		stepResult.Duration = time.Since(stepStarted)
		result.Steps = append(result.Steps, stepResult)
		if err != nil {
			result.Passed = false
			result.Err = fmt.Errorf("%s: %w", step.Name, err)
			break
		}
	}
	result.Duration = time.Since(started)
	return result
}

func interpolateStep(step Step, vars map[string]string) (Step, error) {
	raw, err := json.Marshal(step)
	if err != nil {
		return Step{}, err
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return Step{}, err
	}
	expanded, err := expandValue(generic, vars)
	if err != nil {
		return Step{}, err
	}
	raw, err = json.Marshal(expanded)
	if err != nil {
		return Step{}, err
	}
	var out Step
	if err := json.Unmarshal(raw, &out); err != nil {
		return Step{}, err
	}
	return out, nil
}

var variablePattern = regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)\}`)

func expandString(value string, vars map[string]string) (string, error) {
	var missing string
	expanded := variablePattern.ReplaceAllStringFunc(value, func(match string) string {
		name := match[2 : len(match)-1]
		replacement, ok := vars[name]
		if !ok {
			missing = name
			return match
		}
		return replacement
	})
	if missing != "" {
		return "", fmt.Errorf("undefined variable %q", missing)
	}
	return expanded, nil
}

func expandValue(value any, vars map[string]string) (any, error) {
	switch v := value.(type) {
	case string:
		return expandString(v, vars)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			expanded, err := expandValue(item, vars)
			if err != nil {
				return nil, err
			}
			out[i] = expanded
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			expanded, err := expandValue(item, vars)
			if err != nil {
				return nil, err
			}
			out[key] = expanded
		}
		return out, nil
	default:
		return value, nil
	}
}

func (h *Harness) executeStep(ctx context.Context, step Step) ([]string, error) {
	var logs []string
	var data any
	var status int
	var headers http.Header
	var body []byte

	switch step.Type {
	case "http", "webdav":
		var err error
		data, status, headers, body, err = h.doHTTP(ctx, step)
		logs = append(logs, fmt.Sprintf("%s %s -> %d", strings.ToUpper(step.Method), step.Path, status))
		if err != nil {
			return logs, err
		}
	case "smtp":
		if err := h.doSmtp(step); err != nil {
			return logs, err
		}
		logs = append(logs, fmt.Sprintf("SMTP %s -> %v", step.Port, step.To))
	case "mcp":
		var err error
		data, err = h.doMCP(ctx, step)
		logs = append(logs, "MCP tool "+step.Tool)
		if err != nil {
			return logs, err
		}
	case "sleep":
		time.Sleep(time.Duration(step.Millis) * time.Millisecond)
		logs = append(logs, fmt.Sprintf("slept %dms", step.Millis))
	default:
		return logs, fmt.Errorf("unknown step type %q", step.Type)
	}

	if step.Expect != nil {
		if err := checkExpectation(step.Expect, status, headers, body, data); err != nil {
			return logs, err
		}
		logs = append(logs, "assertions passed")
	}
	if step.Save != nil {
		values, err := saveValues(step.Save, data)
		if err != nil {
			return logs, err
		}
		for key, value := range values {
			h.vars[key] = value
		}
		logs = append(logs, fmt.Sprintf("saved %d variable(s)", len(values)))
	}
	return logs, nil
}

func (h *Harness) doHTTP(ctx context.Context, step Step) (any, int, http.Header, []byte, error) {
	method := strings.ToUpper(step.Method)
	var bodyReader io.Reader
	contentType := ""
	if step.Body != nil {
		encoded, err := json.Marshal(step.Body)
		if err != nil {
			return nil, 0, nil, nil, err
		}
		bodyReader = bytes.NewReader(encoded)
		contentType = "application/json"
	} else if step.BodyRaw != "" {
		bodyReader = strings.NewReader(step.BodyRaw)
		if step.Type == "webdav" {
			contentType = "application/xml"
		} else {
			contentType = "text/plain"
		}
	}

	request, err := http.NewRequestWithContext(ctx, method, h.baseURL+step.Path, bodyReader)
	if err != nil {
		return nil, 0, nil, nil, err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	for key, value := range step.Headers {
		request.Header.Set(key, value)
	}
	if step.Depth != "" {
		request.Header.Set("Depth", step.Depth)
	}
	switch {
	case step.Bearer != "":
		request.Header.Set("Authorization", "Bearer "+step.Bearer)
	case step.Basic != nil:
		request.SetBasicAuth(step.Basic.Username, step.Basic.Password)
	case step.Auth == "session":
		// The cookie jar supplies the session.
	case step.Auth == "" || step.Auth == "none":
	default:
		return nil, 0, nil, nil, fmt.Errorf("unknown auth mode %q", step.Auth)
	}

	response, err := h.client.Do(request)
	if err != nil {
		return nil, 0, nil, nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 20<<20))
	if err != nil {
		return nil, response.StatusCode, response.Header, nil, err
	}
	var parsed any
	if len(responseBody) > 0 {
		_ = json.Unmarshal(responseBody, &parsed)
	}
	return parsed, response.StatusCode, response.Header, responseBody, nil
}

func (h *Harness) doSmtp(step Step) error {
	port := step.Port
	if port == "" {
		port = h.vars["smtp_port"]
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid SMTP port %q", port)
	}
	message, err := buildMessage(step)
	if err != nil {
		return err
	}
	return smtp.SendMail(net.JoinHostPort(h.smtpHost, strconv.Itoa(portNumber)), nil, step.From, step.To, message)
}

func buildMessage(step Step) ([]byte, error) {
	if strings.ContainsAny(step.From+step.Subject+strings.Join(step.To, ""), "\r\n") {
		return nil, errors.New("SMTP addresses and subject must not contain newlines")
	}
	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, "From: %s\r\n", step.From)
	fmt.Fprintf(&buffer, "To: %s\r\n", strings.Join(step.To, ", "))
	subject := mime.QEncoding.Encode("utf-8", step.Subject)
	fmt.Fprintf(&buffer, "Subject: %s\r\n", subject)
	fmt.Fprint(&buffer, "MIME-Version: 1.0\r\n")

	if step.HTML == "" {
		fmt.Fprint(&buffer, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
		buffer.WriteString(step.Text)
		return buffer.Bytes(), nil
	}

	writer := multipart.NewWriter(&buffer)
	textHeader := textproto.MIMEHeader{}
	textHeader.Set("Content-Type", "text/plain; charset=utf-8")
	textPart, err := writer.CreatePart(textHeader)
	if err != nil {
		return nil, err
	}
	_, _ = textPart.Write([]byte(step.Text))
	htmlHeader := textproto.MIMEHeader{}
	htmlHeader.Set("Content-Type", "text/html; charset=utf-8")
	htmlPart, err := writer.CreatePart(htmlHeader)
	if err != nil {
		return nil, err
	}
	_, _ = htmlPart.Write([]byte(step.HTML))
	if err := writer.Close(); err != nil {
		return nil, err
	}

	var final bytes.Buffer
	fmt.Fprintf(&final, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n",
		step.From, strings.Join(step.To, ", "), subject, writer.Boundary())
	final.Write(buffer.Bytes())
	return final.Bytes(), nil
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

func (h *Harness) doMCP(ctx context.Context, step Step) (any, error) {
	key := "session"
	httpClient := h.client
	if step.Bearer != "" {
		key = "bearer:" + step.Bearer
		httpClient = &http.Client{Timeout: 30 * time.Second, Transport: bearerTransport{token: step.Bearer}}
	} else if step.Auth != "" && step.Auth != "session" {
		return nil, fmt.Errorf("MCP requires session auth or a bearer token")
	}

	h.mcpMu.Lock()
	session := h.mcpSessions[key]
	h.mcpMu.Unlock()
	if session == nil {
		client := mcp.NewClient(&mcp.Implementation{Name: "mailvoid-it", Version: "1.0.0"}, nil)
		transport := &mcp.StreamableClientTransport{
			Endpoint:             h.baseURL + "/mcp",
			HTTPClient:           httpClient,
			DisableStandaloneSSE: true,
			MaxRetries:           -1,
		}
		var err error
		session, err = client.Connect(ctx, transport, nil)
		if err != nil {
			return nil, fmt.Errorf("connect MCP: %w", err)
		}
		h.mcpMu.Lock()
		h.mcpSessions[key] = session
		h.mcpMu.Unlock()
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: step.Tool, Arguments: step.Arguments})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("MCP tool returned an error: %s", contentText(result.Content))
	}
	if result.StructuredContent != nil {
		return result.StructuredContent, nil
	}
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			var parsed any
			if json.Unmarshal([]byte(text.Text), &parsed) == nil {
				return parsed, nil
			}
			return text.Text, nil
		}
	}
	return nil, errors.New("MCP tool returned no structured content")
}

func contentText(content []mcp.Content) string {
	var parts []string
	for _, item := range content {
		if text, ok := item.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func checkExpectation(expect *Expectation, status int, headers http.Header, body []byte, data any) error {
	if expect.Status != 0 && status != expect.Status {
		return fmt.Errorf("status = %d, want %d", status, expect.Status)
	}
	bodyText := string(body)
	for _, want := range expect.BodyContains {
		if !strings.Contains(bodyText, want) {
			return fmt.Errorf("body does not contain %q", want)
		}
	}
	for _, unwanted := range expect.BodyNotContains {
		if strings.Contains(bodyText, unwanted) {
			return fmt.Errorf("body unexpectedly contains %q", unwanted)
		}
	}
	for name, want := range expect.Headers {
		got := headers.Get(name)
		if !strings.Contains(got, want) {
			return fmt.Errorf("header %s = %q, want %q", name, got, want)
		}
	}
	if data == nil && len(expect.JSON) > 0 {
		_ = json.Unmarshal(body, &data)
	}
	for _, assertion := range expect.JSON {
		if err := checkJSONAssertion(assertion, data); err != nil {
			return err
		}
	}
	return nil
}

func checkJSONAssertion(assertion JSONAssertion, data any) error {
	value, found := lookupPath(data, assertion.Path)
	if assertion.Exists != nil {
		if *assertion.Exists != found {
			return fmt.Errorf("%s exists = %v, want %v", assertion.Path, found, *assertion.Exists)
		}
	}
	if assertion.Equals != nil {
		if !found {
			return fmt.Errorf("%s not found", assertion.Path)
		}
		if !reflect.DeepEqual(value, *assertion.Equals) {
			return fmt.Errorf("%s = %#v, want %#v", assertion.Path, value, *assertion.Equals)
		}
	}
	if assertion.Contains != "" {
		if !found || !strings.Contains(stringValue(value), assertion.Contains) {
			return fmt.Errorf("%s = %#v does not contain %q", assertion.Path, value, assertion.Contains)
		}
	}
	if assertion.GTE != nil || assertion.LTE != nil {
		number, ok := numericValue(value)
		if !ok {
			return fmt.Errorf("%s is not numeric", assertion.Path)
		}
		if assertion.GTE != nil && number < *assertion.GTE {
			return fmt.Errorf("%s = %v < %v", assertion.Path, number, *assertion.GTE)
		}
		if assertion.LTE != nil && number > *assertion.LTE {
			return fmt.Errorf("%s = %v > %v", assertion.Path, number, *assertion.LTE)
		}
	}
	if assertion.Length != nil {
		length := 0
		switch current := value.(type) {
		case []any:
			length = len(current)
		case string:
			length = len(current)
		case map[string]any:
			length = len(current)
		default:
			return fmt.Errorf("%s has no length", assertion.Path)
		}
		if length != *assertion.Length {
			return fmt.Errorf("%s length = %d, want %d", assertion.Path, length, *assertion.Length)
		}
	}
	return nil
}

func lookupPath(data any, path string) (any, bool) {
	if path == "" {
		return data, true
	}
	current := data
	for _, segment := range strings.Split(path, ".") {
		switch node := current.(type) {
		case map[string]any:
			next, ok := node[segment]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(node) {
				return nil, false
			}
			current = node[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	switch current := value.(type) {
	case string:
		return current
	case float64:
		return strconv.FormatFloat(current, 'f', -1, 64)
	default:
		encoded, _ := json.Marshal(current)
		return string(encoded)
	}
}

func numericValue(value any) (float64, bool) {
	switch current := value.(type) {
	case float64:
		return current, true
	case int:
		return float64(current), true
	case json.Number:
		number, err := current.Float64()
		return number, err == nil
	default:
		return 0, false
	}
}

func saveValues(save map[string]string, data any) (map[string]string, error) {
	out := make(map[string]string, len(save))
	for variable, path := range save {
		value, found := lookupPath(data, path)
		if !found {
			return nil, fmt.Errorf("save %s: path %q not found", variable, path)
		}
		out[variable] = stringValue(value)
	}
	return out, nil
}
