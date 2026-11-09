package policy_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/policy"
	"github.com/gnrfan/mailvoid/internal/storage"
)

func testMessage() *models.Message {
	return &models.Message{
		ID:        "m1",
		RemoteIP:  "198.51.100.7",
		LocalPort: 2525,
		Domain:    "example.test",
		Mailbox:   "inbox",
		Recipient: "inbox@example.test",
		Sender:    "promo@spam.example",
		Subject:   "Buy now",
	}
}

func seed(t *testing.T, store storage.Store, rules []*models.Policy) {
	t.Helper()
	for _, r := range rules {
		if err := store.SavePolicy(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFirstMatchingPolicyWins(t *testing.T) {
	store := storage.NewMemory()
	seed(t, store, []*models.Policy{
		{Name: "drop spam sender", Field: models.FieldSender, Pattern: `@spam\.example$`, Action: models.ActionDrop, Enabled: true},
		{Name: "keep everything", Field: models.FieldAny, Pattern: `.*`, Action: models.ActionKeep, Enabled: true},
	})

	decision, err := policy.New(store).Evaluate(context.Background(), testMessage())
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Matched || decision.Action != models.ActionDrop || decision.PolicyName != "drop spam sender" {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestTTLAndExpiry(t *testing.T) {
	store := storage.NewMemory()
	seed(t, store, []*models.Policy{
		{Name: "expire", Field: models.FieldSubject, Pattern: `(?i)buy now`, Action: models.ActionTTL, TTLSeconds: 3600, Enabled: true},
	})

	msg := testMessage()
	before := time.Now()
	decision, err := policy.New(store).Apply(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != models.ActionTTL {
		t.Fatalf("decision = %+v", decision)
	}
	if msg.Action != models.ActionTTL || msg.ExpiresAt == nil {
		t.Fatalf("message not mutated: %+v", msg)
	}
	if msg.ExpiresAt.Before(before.Add(3599*time.Second)) || msg.ExpiresAt.After(before.Add(3601*time.Second)) {
		t.Fatalf("unexpected expiry: %v", msg.ExpiresAt)
	}
}

func TestNoMatchKeepsMessage(t *testing.T) {
	store := storage.NewMemory()
	seed(t, store, []*models.Policy{
		{Name: "nope", Field: models.FieldDomain, Pattern: `other\.test$`, Action: models.ActionDrop, Enabled: true},
	})
	msg := testMessage()
	decision, err := policy.New(store).Apply(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Matched || decision.Action != models.ActionKeep {
		t.Fatalf("decision = %+v", decision)
	}
	if msg.Action != models.ActionKeep || msg.ExpiresAt != nil {
		t.Fatalf("message = %+v", msg)
	}
}

func TestAllFields(t *testing.T) {
	cases := []struct {
		field models.Message
		name  string
		want  string
	}{
		{name: "sender", field: models.Message{Sender: "s@x"}, want: "s@x"},
		{name: "recipient", field: models.Message{Recipient: "r@x"}, want: "r@x"},
		{name: "mailbox", field: models.Message{Mailbox: "box"}, want: "box"},
		{name: "domain", field: models.Message{Domain: "x.test"}, want: "x.test"},
		{name: "subject", field: models.Message{Subject: "hello"}, want: "hello"},
		{name: "remote", field: models.Message{RemoteIP: "10.0.0.1"}, want: "10.0.0.1"},
		{name: "port", field: models.Message{LocalPort: 1025}, want: "1025"},
	}
	fieldNames := []string{
		models.FieldSender, models.FieldRecipient, models.FieldMailbox,
		models.FieldDomain, models.FieldSubject, models.FieldRemoteIP, models.FieldLocalPort,
	}
	for i, tc := range cases {
		value := policy.FieldValue(&tc.field, fieldNames[i])
		if value != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, value, tc.want)
		}
	}
}

func TestDisabledPolicyIsIgnored(t *testing.T) {
	store := storage.NewMemory()
	seed(t, store, []*models.Policy{
		{Name: "disabled", Field: models.FieldAny, Pattern: `.*`, Action: models.ActionDrop, Enabled: false},
	})
	decision, err := policy.New(store).Evaluate(context.Background(), testMessage())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Matched {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestInvalidRegexIsSkipped(t *testing.T) {
	store := storage.NewMemory()
	if err := store.SavePolicy(context.Background(), &models.Policy{
		Name: "bad", Field: models.FieldAny, Pattern: "([", Action: models.ActionDrop, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	decision, err := policy.New(store).Evaluate(context.Background(), testMessage())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Matched {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestValidate(t *testing.T) {
	valid := &models.Policy{Name: "ok", Field: models.FieldSender, Pattern: `@test$`, Action: models.ActionTTL, TTLSeconds: 60}
	if err := policy.Validate(valid); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
	cases := []*models.Policy{
		{Name: "", Field: models.FieldSender, Pattern: "x", Action: models.ActionKeep},
		{Name: "bad field", Field: "nope", Pattern: "x", Action: models.ActionKeep},
		{Name: "bad action", Field: models.FieldSender, Pattern: "x", Action: "nope"},
		{Name: "bad regex", Field: models.FieldSender, Pattern: "([", Action: models.ActionKeep},
		{Name: "no ttl", Field: models.FieldSender, Pattern: "x", Action: models.ActionTTL},
	}
	for _, p := range cases {
		if err := policy.Validate(p); err == nil {
			t.Errorf("expected error for %+v", p)
		}
	}
}

func TestTestPattern(t *testing.T) {
	matched, err := policy.TestPattern(models.FieldSubject, "hello", "well hello there")
	if err != nil || !matched {
		t.Fatalf("matched=%v err=%v", matched, err)
	}
	if _, err := policy.TestPattern(models.FieldSubject, "([", "x"); err == nil {
		t.Fatal("expected regex error")
	}
}

func TestFieldValueAnyAndUnknown(t *testing.T) {
	msg := testMessage()
	any := policy.FieldValue(msg, models.FieldAny)
	for _, want := range []string{"promo@spam.example", "inbox@example.test", "inbox", "example.test", "Buy now", "198.51.100.7", "2525"} {
		if !strings.Contains(any, want) {
			t.Errorf("any value %q missing %q", any, want)
		}
	}
	if got := policy.FieldValue(msg, "unknown"); got != "" {
		t.Fatalf("unknown = %q", got)
	}
}

func TestRegexCacheAndInvalidCache(t *testing.T) {
	store := storage.NewMemory()
	if err := store.SavePolicy(context.Background(), &models.Policy{Name: "ok", Field: models.FieldAny, Pattern: ".*", Action: models.ActionKeep, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	engine := policy.New(store)
	for i := 0; i < 3; i++ {
		if decision, err := engine.Evaluate(context.Background(), testMessage()); err != nil || !decision.Matched {
			t.Fatalf("iteration %d: %+v %v", i, decision, err)
		}
	}

	store = storage.NewMemory()
	if err := store.SavePolicy(context.Background(), &models.Policy{Name: "bad", Field: models.FieldAny, Pattern: "([", Action: models.ActionDrop, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	engine = policy.New(store)
	for i := 0; i < 2; i++ {
		if decision, err := engine.Evaluate(context.Background(), testMessage()); err != nil || decision.Matched {
			t.Fatalf("invalid iteration %d: %+v %v", i, decision, err)
		}
	}
}

func TestValidateMore(t *testing.T) {
	long := &models.Policy{Name: "long", Field: models.FieldAny, Pattern: strings.Repeat("a", 2049), Action: models.ActionKeep}
	if err := policy.Validate(long); err == nil {
		t.Fatal("expected pattern length error")
	}
	normalized := &models.Policy{Name: "keep", Field: models.FieldAny, Pattern: ".*", Action: models.ActionKeep, TTLSeconds: 60}
	if err := policy.Validate(normalized); err != nil {
		t.Fatal(err)
	}
	if normalized.TTLSeconds != 0 {
		t.Fatalf("ttl should be normalized: %d", normalized.TTLSeconds)
	}
	emptyPattern := &models.Policy{Name: "empty", Field: models.FieldAny, Pattern: "", Action: models.ActionKeep}
	if err := policy.Validate(emptyPattern); err == nil {
		t.Fatal("expected empty pattern error")
	}
	ttlZero := &models.Policy{Name: "ttl", Field: models.FieldAny, Pattern: ".*", Action: models.ActionTTL}
	if err := policy.Validate(ttlZero); err == nil {
		t.Fatal("expected ttl error")
	}
	if _, err := policy.TestPattern("unknown", ".*", "x"); err == nil {
		t.Fatal("expected field error")
	}
}

type failingPolicyStore struct {
	storage.Store
	err error
}

func (f failingPolicyStore) ListPolicies(context.Context) ([]*models.Policy, error) {
	return nil, f.err
}

func TestEngineStoreErrors(t *testing.T) {
	engine := policy.New(failingPolicyStore{Store: storage.NewMemory(), err: errors.New("boom")})
	if _, err := engine.Evaluate(context.Background(), testMessage()); err == nil {
		t.Fatal("expected evaluate error")
	}
	if _, err := engine.Apply(context.Background(), testMessage()); err == nil {
		t.Fatal("expected apply error")
	}
}
