package smtpserver_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/config"
	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/policy"
	"github.com/gnrfan/mailvoid/internal/smtpserver"
	"github.com/gnrfan/mailvoid/internal/storage"
)

func testConfig(ports ...int) config.Config {
	cfg := config.Default()
	cfg.SMTPHost = "127.0.0.1"
	cfg.SMTPPorts = ports
	cfg.Domains = []string{"*"}
	cfg.MaxMessageBytes = 1 << 20
	return cfg
}

func startServer(t *testing.T, cfg config.Config) (*smtpserver.Server, storage.Store, *events.Bus) {
	t.Helper()
	store := storage.NewMemory()
	srv, bus := startServerWithStore(t, cfg, store)
	return srv, store, bus
}

func startServerWithStore(t *testing.T, cfg config.Config, store storage.Store) (*smtpserver.Server, *events.Bus) {
	t.Helper()
	bus := events.NewBus(store)
	engine := policy.New(store)
	srv := smtpserver.New(cfg, store, engine, bus)
	if err := srv.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv, bus
}

const sampleMessage = "From: sender@example.org\r\n" +
	"To: inbox@example.test\r\n" +
	"Subject: Test subject\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"Hello from the test.\r\n"

func TestCaptureMessage(t *testing.T) {
	srv, store, bus := startServer(t, testConfig(0))
	addr := srv.Addrs()[0].String()

	if err := smtp.SendMail(addr, nil, "sender@example.org", []string{"inbox@example.test"}, []byte(sampleMessage)); err != nil {
		t.Fatalf("send: %v", err)
	}

	msgs, err := store.ListMessages(context.Background(), models.MessageFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	msg := msgs[0]
	if msg.RemoteIP != "127.0.0.1" {
		t.Errorf("remote ip = %q", msg.RemoteIP)
	}
	_, portStr, _ := net.SplitHostPort(addr)
	wantPort, _ := strconv.Atoi(portStr)
	if msg.LocalPort != wantPort {
		t.Errorf("local port = %d, want %d", msg.LocalPort, wantPort)
	}
	if msg.Sender != "sender@example.org" || msg.Mailbox != "inbox" || msg.Domain != "example.test" {
		t.Errorf("envelope = %+v", msg)
	}
	if msg.Subject != "Test subject" {
		t.Errorf("subject = %q", msg.Subject)
	}
	if !strings.Contains(msg.BodyText, "Hello from the test.") {
		t.Errorf("body = %q", msg.BodyText)
	}
	if msg.Size == 0 || msg.Raw == "" {
		t.Errorf("raw not stored: %+v", msg)
	}

	types := make(map[string]bool)
	for _, ev := range bus.Recent(0) {
		types[ev.Type] = true
	}
	for _, want := range []string{
		events.TypeSMTPConnected, events.TypeSMTPMailFrom, events.TypeSMTPRcptTo,
		events.TypeSMTPData, events.TypeMessageReceived,
	} {
		if !types[want] {
			t.Errorf("missing event %s", want)
		}
	}
}

func TestMultiplePorts(t *testing.T) {
	srv, store, _ := startServer(t, testConfig(0, 0))
	addrs := srv.Addrs()
	if len(addrs) != 2 {
		t.Fatalf("addrs = %v", addrs)
	}
	for _, addr := range addrs {
		if err := smtp.SendMail(addr.String(), nil, "a@b.test", []string{"x@y.test"}, []byte(sampleMessage)); err != nil {
			t.Fatalf("send to %s: %v", addr, err)
		}
	}
	msgs, _ := store.ListMessages(context.Background(), models.MessageFilter{})
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2", len(msgs))
	}
	if msgs[0].LocalPort == msgs[1].LocalPort {
		t.Fatalf("expected distinct receiving ports: %d", msgs[0].LocalPort)
	}
}

func TestDomainRejection(t *testing.T) {
	cfg := testConfig(0)
	cfg.Domains = []string{"allowed.test"}
	srv, store, bus := startServer(t, cfg)
	addr := srv.Addrs()[0].String()

	err := smtp.SendMail(addr, nil, "a@b.test", []string{"x@denied.test"}, []byte(sampleMessage))
	if err == nil {
		t.Fatal("expected rejection")
	}
	msgs, _ := store.ListMessages(context.Background(), models.MessageFilter{})
	if len(msgs) != 0 {
		t.Fatalf("messages = %d, want 0", len(msgs))
	}
	found := false
	for _, ev := range bus.Recent(0) {
		if ev.Type == events.TypeSMTPRcptTo && ev.Payload["accepted"] == false {
			found = true
		}
	}
	if !found {
		t.Fatal("expected rejected rcpt event")
	}
}

func TestDropPolicy(t *testing.T) {
	srv, store, bus := startServer(t, testConfig(0))
	if err := store.SavePolicy(context.Background(), &models.Policy{
		Name: "drop spam", Field: models.FieldSender, Pattern: `@spam\.test$`,
		Action: models.ActionDrop, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	addr := srv.Addrs()[0].String()
	if err := smtp.SendMail(addr, nil, "promo@spam.test", []string{"inbox@example.test"}, []byte(sampleMessage)); err != nil {
		t.Fatalf("send: %v", err)
	}
	msgs, _ := store.ListMessages(context.Background(), models.MessageFilter{})
	if len(msgs) != 0 {
		t.Fatalf("dropped message was stored: %d", len(msgs))
	}
	found := false
	for _, ev := range bus.Recent(0) {
		if ev.Type == events.TypeMessageDropped {
			found = true
		}
	}
	if !found {
		t.Fatal("expected message.dropped event")
	}
}

func TestTTLPolicy(t *testing.T) {
	srv, store, _ := startServer(t, testConfig(0))
	if err := store.SavePolicy(context.Background(), &models.Policy{
		Name: "expire", Field: models.FieldSubject, Pattern: "Test subject",
		Action: models.ActionTTL, TTLSeconds: 60, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	addr := srv.Addrs()[0].String()
	if err := smtp.SendMail(addr, nil, "sender@example.org", []string{"inbox@example.test"}, []byte(sampleMessage)); err != nil {
		t.Fatalf("send: %v", err)
	}
	msgs, _ := store.ListMessages(context.Background(), models.MessageFilter{})
	if len(msgs) != 1 {
		t.Fatalf("messages = %d", len(msgs))
	}
	if msgs[0].Action != models.ActionTTL || msgs[0].ExpiresAt == nil {
		t.Fatalf("message = %+v", msgs[0])
	}
}

func TestMultipleRecipients(t *testing.T) {
	srv, store, _ := startServer(t, testConfig(0))
	addr := srv.Addrs()[0].String()
	if err := smtp.SendMail(addr, nil, "sender@example.org", []string{"one@a.test", "two@b.test"}, []byte(sampleMessage)); err != nil {
		t.Fatalf("send: %v", err)
	}
	msgs, _ := store.ListMessages(context.Background(), models.MessageFilter{})
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2", len(msgs))
	}
	mailboxes := map[string]bool{}
	for _, m := range msgs {
		mailboxes[m.Mailbox+"@"+m.Domain] = true
	}
	if !mailboxes["one@a.test"] || !mailboxes["two@b.test"] {
		t.Fatalf("mailboxes = %v", mailboxes)
	}
}

func TestUnparsableMessageIsStillStored(t *testing.T) {
	srv, store, bus := startServer(t, testConfig(0))
	addr := srv.Addrs()[0].String()
	if err := smtp.SendMail(addr, nil, "sender@example.org", []string{"inbox@example.test"}, []byte("this is not a valid message")); err != nil {
		t.Fatalf("send: %v", err)
	}
	msgs, _ := store.ListMessages(context.Background(), models.MessageFilter{})
	if len(msgs) != 1 {
		t.Fatalf("messages = %d", len(msgs))
	}
	if msgs[0].Subject != "(unparsable message)" {
		t.Fatalf("subject = %q", msgs[0].Subject)
	}
	found := false
	for _, ev := range bus.Recent(0) {
		if ev.Type == events.TypeSMTPError {
			found = true
		}
	}
	if !found {
		t.Fatal("expected smtp.error for unparsable message")
	}
}

type failingSaveStore struct {
	storage.Store
	err error
}

func (f failingSaveStore) SaveMessage(context.Context, *models.Message) error { return f.err }

type failingPoliciesStore struct {
	storage.Store
	err error
}

func (f failingPoliciesStore) ListPolicies(context.Context) ([]*models.Policy, error) {
	return nil, f.err
}

func rawSMTP(t *testing.T, addr string, commands ...string) []string {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	read := func() string {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		return strings.TrimSpace(line)
	}
	greeting := read()
	if !strings.HasPrefix(greeting, "220") {
		t.Fatalf("greeting = %q", greeting)
	}
	var responses []string
	for _, command := range commands {
		if _, err := fmt.Fprintf(conn, "%s\r\n", command); err != nil {
			t.Fatal(err)
		}
		responses = append(responses, read())
	}
	return responses
}

func TestInvalidEnvelopeCommands(t *testing.T) {
	srv, _, _ := startServer(t, testConfig(0))
	addr := srv.Addrs()[0].String()

	responses := rawSMTP(t, addr,
		"HELO test",
		"MAIL FROM:<sender@example.org>",
		"RCPT TO:<not-an-address>",
		"QUIT",
	)
	if len(responses) != 4 {
		t.Fatalf("responses = %+v", responses)
	}
	if !strings.HasPrefix(responses[1], "250") {
		t.Fatalf("MAIL response = %q", responses[1])
	}
	if !strings.HasPrefix(responses[2], "501") {
		t.Fatalf("RCPT response = %q", responses[2])
	}

	responses = rawSMTP(t, addr, "HELO test", "MAIL FROM:<bad>", "QUIT")
	if !strings.HasPrefix(responses[1], "501") {
		t.Fatalf("bad MAIL response = %q", responses[1])
	}
}

func TestEmptyEnvelopeSender(t *testing.T) {
	srv, store, _ := startServer(t, testConfig(0))
	client, err := smtp.Dial(srv.Addrs()[0].String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Mail(""); err != nil {
		t.Fatal(err)
	}
	if err := client.Rcpt("inbox@example.test"); err != nil {
		t.Fatal(err)
	}
	writer, err := client.Data()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(sampleMessage)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Quit(); err != nil {
		t.Fatal(err)
	}

	messages, _ := store.ListMessages(context.Background(), models.MessageFilter{})
	if len(messages) != 1 || messages[0].Sender != "" {
		t.Fatalf("messages = %+v", messages)
	}
}

func TestOversizedMessage(t *testing.T) {
	cfg := testConfig(0)
	cfg.MaxMessageBytes = 64
	srv, store, _ := startServer(t, cfg)
	err := smtp.SendMail(srv.Addrs()[0].String(), nil, "sender@example.org", []string{"inbox@example.test"}, []byte(sampleMessage))
	if err == nil {
		t.Fatal("expected oversized error")
	}
	messages, _ := store.ListMessages(context.Background(), models.MessageFilter{})
	if len(messages) != 0 {
		t.Fatalf("oversized message stored: %+v", messages)
	}
}

func TestStorageFailureReturns451(t *testing.T) {
	cfg := testConfig(0)
	store := failingSaveStore{Store: storage.NewMemory(), err: errors.New("disk full")}
	srv, bus := startServerWithStore(t, cfg, store)
	err := smtp.SendMail(srv.Addrs()[0].String(), nil, "sender@example.org", []string{"inbox@example.test"}, []byte(sampleMessage))
	if err == nil || !strings.Contains(err.Error(), "451") {
		t.Fatalf("send error = %v", err)
	}
	found := false
	for _, event := range bus.Recent(0) {
		if event.Type == events.TypeMessageError {
			found = true
		}
	}
	if !found {
		t.Fatal("expected message.error event")
	}
}

func TestPolicyFailureKeepsMessage(t *testing.T) {
	cfg := testConfig(0)
	memory := storage.NewMemory()
	store := failingPoliciesStore{Store: memory, err: errors.New("policy store down")}
	srv, bus := startServerWithStore(t, cfg, store)
	if err := smtp.SendMail(srv.Addrs()[0].String(), nil, "sender@example.org", []string{"inbox@example.test"}, []byte(sampleMessage)); err != nil {
		t.Fatal(err)
	}
	messages, _ := memory.ListMessages(context.Background(), models.MessageFilter{})
	if len(messages) != 1 || messages[0].Action != models.ActionKeep {
		t.Fatalf("messages = %+v", messages)
	}
	found := false
	for _, event := range bus.Recent(0) {
		if event.Type == events.TypeSMTPError {
			found = true
		}
	}
	if !found {
		t.Fatal("expected smtp.error event")
	}
}

func TestDomainPatterns(t *testing.T) {
	cfg := testConfig(0)
	cfg.Domains = []string{"allowed.test", "*.zone.test"}
	srv, store, _ := startServer(t, cfg)
	addr := srv.Addrs()[0].String()
	for _, recipient := range []string{"a@allowed.test", "b@sub.zone.test", "c@zone.test"} {
		if err := smtp.SendMail(addr, nil, "sender@example.org", []string{recipient}, []byte(sampleMessage)); err != nil {
			t.Fatalf("%s: %v", recipient, err)
		}
	}
	messages, _ := store.ListMessages(context.Background(), models.MessageFilter{})
	if len(messages) != 3 {
		t.Fatalf("messages = %d", len(messages))
	}
}

func TestServerLifecycleAndString(t *testing.T) {
	cfg := testConfig(0)
	srv := smtpserver.New(cfg, storage.NewMemory(), policy.New(storage.NewMemory()), events.NewBus())
	if !strings.Contains(srv.String(), "smtpserver") {
		t.Fatalf("string = %q", srv.String())
	}
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("second close = %v", err)
	}

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port
	cfg = testConfig(port)
	srv = smtpserver.New(cfg, storage.NewMemory(), policy.New(storage.NewMemory()), events.NewBus())
	if err := srv.Start(); err == nil {
		t.Fatal("expected bind error")
	}
}
