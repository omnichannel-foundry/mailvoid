package storage_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/storage"
)

func stores(t *testing.T) map[string]storage.Store {
	t.Helper()
	genjiStore, err := storage.NewGenji(filepath.Join(t.TempDir(), "mailvoid.db"))
	if err != nil {
		t.Fatalf("open genji: %v", err)
	}
	t.Cleanup(func() { _ = genjiStore.Close() })
	return map[string]storage.Store{
		"memory": storage.NewMemory(),
		"genji":  genjiStore,
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestMessageRoundTrip(t *testing.T) {
	ctx := context.Background()
	for name, store := range stores(t) {
		t.Run(name, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Millisecond)
			exp := now.Add(time.Hour)
			msg := &models.Message{
				ID:         "msg_test",
				ReceivedAt: now,
				RemoteIP:   "203.0.113.9",
				LocalPort:  2525,
				Domain:     "example.test",
				Mailbox:    "inbox",
				Recipient:  "inbox@example.test",
				Sender:     "sender@example.org",
				Subject:    "A subject",
				Headers:    map[string][]string{"X-Test": {"one", "two"}},
				RawHeaders: "X-Test: one\r\n",
				Raw:        "raw message",
				BodyText:   "hello",
				BodyHTML:   "<p>hello</p>",
				Size:       11,
				PolicyID:   "pol_1",
				PolicyName: "retain",
				Action:     models.ActionTTL,
				ExpiresAt:  ptrTime(exp),
			}
			if err := store.SaveMessage(ctx, msg); err != nil {
				t.Fatal(err)
			}
			got, err := store.GetMessage(ctx, msg.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Subject != msg.Subject || got.Raw != msg.Raw || got.LocalPort != 2525 {
				t.Fatalf("round trip mismatch: %+v", got)
			}
			if len(got.Headers["X-Test"]) != 2 {
				t.Fatalf("headers = %+v", got.Headers)
			}
			if got.ExpiresAt == nil || !got.ExpiresAt.Equal(exp) {
				t.Fatalf("expires = %v, want %v", got.ExpiresAt, exp)
			}

			msg.Subject = "updated"
			msg.Action = models.ActionKeep
			msg.ExpiresAt = nil
			if err := store.SaveMessage(ctx, msg); err != nil {
				t.Fatal(err)
			}
			got, _ = store.GetMessage(ctx, msg.ID)
			if got.Subject != "updated" || got.Action != models.ActionKeep || got.ExpiresAt != nil {
				t.Fatalf("update failed: %+v", got)
			}

			if _, err := store.GetMessage(ctx, "missing"); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("missing error = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestMessageFiltersAndPagination(t *testing.T) {
	ctx := context.Background()
	for name, store := range stores(t) {
		t.Run(name, func(t *testing.T) {
			base := time.Now().UTC().Truncate(time.Millisecond)
			msgs := []*models.Message{
				{ID: "m1", ReceivedAt: base.Add(-3 * time.Minute), Domain: "a.test", Mailbox: "box", Recipient: "box@a.test", Sender: "one@x.test", Subject: "Alpha", RemoteIP: "1.1.1.1", LocalPort: 1025, Action: models.ActionKeep},
				{ID: "m2", ReceivedAt: base.Add(-2 * time.Minute), Domain: "b.test", Mailbox: "other", Recipient: "other@b.test", Sender: "two@x.test", Subject: "Beta", RemoteIP: "2.2.2.2", LocalPort: 2525, Action: models.ActionDrop},
				{ID: "m3", ReceivedAt: base.Add(-1 * time.Minute), Domain: "a.test", Mailbox: "box", Recipient: "box@a.test", Sender: "three@y.test", Subject: "Gamma", RemoteIP: "3.3.3.3", LocalPort: 1025, Action: models.ActionTTL},
			}
			for _, m := range msgs {
				if err := store.SaveMessage(ctx, m); err != nil {
					t.Fatal(err)
				}
			}

			all, err := store.ListMessages(ctx, models.MessageFilter{})
			if err != nil {
				t.Fatal(err)
			}
			if len(all) != 3 || all[0].ID != "m3" {
				t.Fatalf("expected newest-first 3 messages, got %+v", all)
			}

			byDomain, _ := store.ListMessages(ctx, models.MessageFilter{Domain: "A.TEST"})
			if len(byDomain) != 2 {
				t.Fatalf("domain filter got %d", len(byDomain))
			}
			byPort, _ := store.ListMessages(ctx, models.MessageFilter{Port: 2525})
			if len(byPort) != 1 || byPort[0].ID != "m2" {
				t.Fatalf("port filter got %+v", byPort)
			}
			byQuery, _ := store.ListMessages(ctx, models.MessageFilter{Query: "gamma"})
			if len(byQuery) != 1 || byQuery[0].ID != "m3" {
				t.Fatalf("query filter got %+v", byQuery)
			}
			byAction, _ := store.ListMessages(ctx, models.MessageFilter{Action: models.ActionTTL})
			if len(byAction) != 1 || byAction[0].ID != "m3" {
				t.Fatalf("action filter got %+v", byAction)
			}
			page, _ := store.ListMessages(ctx, models.MessageFilter{Limit: 1, Offset: 1})
			if len(page) != 1 || page[0].ID != "m2" {
				t.Fatalf("pagination got %+v", page)
			}
			count, _ := store.CountMessages(ctx, models.MessageFilter{Domain: "a.test"})
			if count != 2 {
				t.Fatalf("count = %d, want 2", count)
			}

			if err := store.DeleteMessage(ctx, "m1"); err != nil {
				t.Fatal(err)
			}
			if _, err := store.GetMessage(ctx, "m1"); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("delete failed: %v", err)
			}
		})
	}
}

func TestDeleteExpired(t *testing.T) {
	ctx := context.Background()
	for name, store := range stores(t) {
		t.Run(name, func(t *testing.T) {
			now := time.Now().UTC()
			expired := &models.Message{ID: "old", ReceivedAt: now.Add(-2 * time.Hour), ExpiresAt: ptrTime(now.Add(-time.Hour)), Action: models.ActionTTL}
			future := &models.Message{ID: "new", ReceivedAt: now, ExpiresAt: ptrTime(now.Add(time.Hour)), Action: models.ActionTTL}
			keep := &models.Message{ID: "keep", ReceivedAt: now, Action: models.ActionKeep}
			for _, m := range []*models.Message{expired, future, keep} {
				if err := store.SaveMessage(ctx, m); err != nil {
					t.Fatal(err)
				}
			}
			got, err := store.DeleteExpired(ctx, now)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0].ID != "old" {
				t.Fatalf("expired = %+v", got)
			}
			if _, err := store.GetMessage(ctx, "old"); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("expected expired removed: %v", err)
			}
			if _, err := store.GetMessage(ctx, "new"); err != nil {
				t.Fatalf("future should remain: %v", err)
			}
			if _, err := store.GetMessage(ctx, "keep"); err != nil {
				t.Fatalf("keep should remain: %v", err)
			}
		})
	}
}

func TestPolicyCRUDAndOrdering(t *testing.T) {
	ctx := context.Background()
	for name, store := range stores(t) {
		t.Run(name, func(t *testing.T) {
			first := &models.Policy{Name: "first", Field: models.FieldSender, Pattern: `@spam\.`, Action: models.ActionDrop, Enabled: true}
			second := &models.Policy{Name: "second", Field: models.FieldSubject, Pattern: "hello", Action: models.ActionTTL, TTLSeconds: 60, Enabled: true}
			for _, p := range []*models.Policy{first, second} {
				if err := store.SavePolicy(ctx, p); err != nil {
					t.Fatal(err)
				}
			}
			policies, err := store.ListPolicies(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(policies) != 2 || policies[0].Name != "first" || policies[1].Name != "second" {
				t.Fatalf("policies = %+v", policies)
			}
			if policies[0].Priority != 0 || policies[1].Priority != 1 {
				t.Fatalf("first policy priority should be 0-based: %+v", policies)
			}
			if err := store.ReorderPolicies(ctx, []string{second.ID, first.ID}); err != nil {
				t.Fatal(err)
			}
			policies, _ = store.ListPolicies(ctx)
			if policies[0].Name != "second" {
				t.Fatalf("reorder failed: %+v", policies)
			}

			first.Pattern = `@evil\.`
			first.Enabled = false
			if err := store.SavePolicy(ctx, first); err != nil {
				t.Fatal(err)
			}
			got, _ := store.GetPolicy(ctx, first.ID)
			if got.Pattern != `@evil\.` || got.Enabled {
				t.Fatalf("update failed: %+v", got)
			}

			if err := store.DeletePolicy(ctx, second.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := store.GetPolicy(ctx, second.ID); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("delete failed: %v", err)
			}
		})
	}
}

func TestEventPersistence(t *testing.T) {
	ctx := context.Background()
	for name, store := range stores(t) {
		t.Run(name, func(t *testing.T) {
			now := time.Now().UTC()
			sink := events.Sink(store)
			if err := sink.Write(events.Event{ID: "e1", Timestamp: now.Add(-time.Minute), Type: events.TypeSMTPConnected, Severity: events.SeverityInfo, Payload: map[string]any{"remote_ip": "127.0.0.1"}}); err != nil {
				t.Fatal(err)
			}
			if err := sink.Write(events.Event{ID: "e2", Timestamp: now, Type: events.TypePolicyMatched, Severity: events.SeverityWarn, Payload: map[string]any{"policy": "spam"}}); err != nil {
				t.Fatal(err)
			}
			list, err := store.ListEvents(ctx, models.EventFilter{})
			if err != nil {
				t.Fatal(err)
			}
			if len(list) != 2 || list[0].ID != "e2" {
				t.Fatalf("events = %+v", list)
			}
			filtered, _ := store.ListEvents(ctx, models.EventFilter{Type: events.TypeSMTPConnected})
			if len(filtered) != 1 || filtered[0].ID != "e1" {
				t.Fatalf("filtered = %+v", filtered)
			}
			search, _ := store.ListEvents(ctx, models.EventFilter{Query: "spam"})
			if len(search) != 1 || search[0].ID != "e2" {
				t.Fatalf("search = %+v", search)
			}
		})
	}
}

func TestUserPersistence(t *testing.T) {
	ctx := context.Background()
	for name, store := range stores(t) {
		t.Run(name, func(t *testing.T) {
			user := &models.User{Username: "Admin", PasswordHash: "hash"}
			if err := store.SaveUser(ctx, user); err != nil {
				t.Fatal(err)
			}
			got, err := store.GetUser(ctx, "admin")
			if err != nil {
				t.Fatal(err)
			}
			if got.Username != "admin" || got.PasswordHash != "hash" {
				t.Fatalf("user = %+v", got)
			}
			user.PasswordHash = "hash2"
			if err := store.SaveUser(ctx, user); err != nil {
				t.Fatal(err)
			}
			got, _ = store.GetUser(ctx, "admin")
			if got.PasswordHash != "hash2" {
				t.Fatalf("update failed: %+v", got)
			}
			if _, err := store.GetUser(ctx, "missing"); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("missing user error = %v", err)
			}
			if err := store.SaveUser(ctx, &models.User{Username: "zeta", PasswordHash: "hash"}); err != nil {
				t.Fatal(err)
			}
			users, err := store.ListUsers(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(users) != 2 || users[0].Username != "admin" || users[1].Username != "zeta" {
				t.Fatalf("users = %+v", users)
			}
		})
	}
}

func TestCountEventsAndEventFilters(t *testing.T) {
	ctx := context.Background()
	for name, store := range stores(t) {
		t.Run(name, func(t *testing.T) {
			now := time.Now().UTC()
			records := []events.Event{
				{ID: "e1", Timestamp: now.Add(-3 * time.Minute), Type: events.TypeSMTPConnected, Severity: events.SeverityInfo, Payload: map[string]any{"remote_ip": "10.0.0.1"}},
				{ID: "e2", Timestamp: now.Add(-2 * time.Minute), Type: events.TypePolicyMatched, Severity: events.SeverityWarn, Payload: map[string]any{"policy": "spam", "enabled": true}},
				{ID: "e3", Timestamp: now.Add(-1 * time.Minute), Type: events.TypeSMTPConnected, Severity: events.SeverityError},
			}
			for _, event := range records {
				if err := store.Write(event); err != nil {
					t.Fatal(err)
				}
			}
			cases := []struct {
				name   string
				filter models.EventFilter
				want   int
			}{
				{"all", models.EventFilter{}, 3},
				{"type", models.EventFilter{Type: events.TypeSMTPConnected}, 2},
				{"severity", models.EventFilter{Severity: events.SeverityWarn}, 1},
				{"query", models.EventFilter{Query: "spam"}, 1},
				{"query bool", models.EventFilter{Query: "true"}, 1},
				{"since", models.EventFilter{Since: now.Add(-150 * time.Second)}, 2},
				{"combined", models.EventFilter{Type: events.TypeSMTPConnected, Severity: events.SeverityError}, 1},
			}
			for _, tc := range cases {
				got, err := store.CountEvents(ctx, tc.filter)
				if err != nil {
					t.Fatalf("%s count: %v", tc.name, err)
				}
				if got != tc.want {
					t.Errorf("%s count = %d, want %d", tc.name, got, tc.want)
				}
			}
			list, err := store.ListEvents(ctx, models.EventFilter{Severity: events.SeverityWarn, Limit: 10, Offset: 0})
			if err != nil || len(list) != 1 || list[0].ID != "e2" {
				t.Fatalf("list = %+v err=%v", list, err)
			}
			paged, err := store.ListEvents(ctx, models.EventFilter{Limit: 1, Offset: 1})
			if err != nil || len(paged) != 1 || paged[0].ID != "e2" {
				t.Fatalf("paged = %+v err=%v", paged, err)
			}
		})
	}
}

func TestMessageFilterEdgesAndGeneratedIDs(t *testing.T) {
	ctx := context.Background()
	for name, store := range stores(t) {
		t.Run(name, func(t *testing.T) {
			base := time.Now().UTC().Truncate(time.Millisecond)
			generated := &models.Message{
				ReceivedAt: base, Domain: "Edge.Test", Mailbox: "Box",
				Recipient: "Box@Edge.Test", Sender: "sender@example.org",
				Subject: "CaseSensitive Subject", Action: models.ActionKeep,
				Deleted: true,
			}
			if err := store.SaveMessage(ctx, generated); err != nil {
				t.Fatal(err)
			}
			if generated.ID == "" {
				t.Fatal("expected generated id")
			}
			defaultList, err := store.ListMessages(ctx, models.MessageFilter{})
			if err != nil || len(defaultList) != 0 {
				t.Fatalf("deleted should be hidden: %+v err=%v", defaultList, err)
			}
			included, err := store.ListMessages(ctx, models.MessageFilter{IncludeDeleted: true})
			if err != nil || len(included) != 1 {
				t.Fatalf("include deleted = %+v err=%v", included, err)
			}
			for _, filter := range []models.MessageFilter{
				{Query: "casesensitive", IncludeDeleted: true},
				{Query: "CASESENSITIVE", IncludeDeleted: true},
				{IncludeDeleted: true, Domain: "edge.test"},
				{IncludeDeleted: true, Mailbox: "box"},
				{IncludeDeleted: true, Action: models.ActionKeep},
				{IncludeDeleted: true, Since: base.Add(-time.Second)},
				{IncludeDeleted: true, Until: base.Add(time.Second)},
				{IncludeDeleted: true, Offset: -10},
			} {
				list, err := store.ListMessages(ctx, filter)
				if err != nil {
					t.Fatalf("filter %+v: %v", filter, err)
				}
				if len(list) != 1 {
					t.Fatalf("filter %+v returned %d", filter, len(list))
				}
			}
			if list, _ := store.ListMessages(ctx, models.MessageFilter{IncludeDeleted: true, Since: base.Add(time.Minute)}); len(list) != 0 {
				t.Fatalf("since future returned %d", len(list))
			}
			if list, _ := store.ListMessages(ctx, models.MessageFilter{IncludeDeleted: true, Until: base.Add(-time.Minute)}); len(list) != 0 {
				t.Fatalf("until past returned %d", len(list))
			}
			if count, _ := store.CountMessages(ctx, models.MessageFilter{IncludeDeleted: true}); count != 1 {
				t.Fatalf("count = %d", count)
			}
			if expired, err := store.DeleteExpired(ctx, time.Now().UTC()); err != nil || len(expired) != 0 {
				t.Fatalf("delete expired = %+v err=%v", expired, err)
			}
			if err := store.DeleteMessage(ctx, "missing"); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("delete missing = %v", err)
			}
		})
	}
}

func TestPolicyEdges(t *testing.T) {
	ctx := context.Background()
	for name, store := range stores(t) {
		t.Run(name, func(t *testing.T) {
			if _, err := store.GetPolicy(ctx, "missing"); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("get missing = %v", err)
			}
			if err := store.DeletePolicy(ctx, "missing"); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("delete missing = %v", err)
			}
			if err := store.ReorderPolicies(ctx, []string{"missing"}); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("reorder missing = %v", err)
			}
			first := &models.Policy{Name: "first", Field: models.FieldSender, Pattern: "a", Action: models.ActionKeep, Enabled: true, Priority: 5}
			if err := store.SavePolicy(ctx, first); err != nil {
				t.Fatal(err)
			}
			if first.Priority != 5 {
				t.Fatalf("explicit priority = %d", first.Priority)
			}
			created := first.CreatedAt
			first.Name = "updated"
			first.Enabled = false
			if err := store.SavePolicy(ctx, first); err != nil {
				t.Fatal(err)
			}
			updated, err := store.GetPolicy(ctx, first.ID)
			if err != nil {
				t.Fatal(err)
			}
			if updated.Name != "updated" || updated.Enabled || updated.CreatedAt.UnixMilli() != created.UnixMilli() || updated.Priority != 5 {
				t.Fatalf("updated = %+v", updated)
			}
			second := &models.Policy{Name: "second", Field: models.FieldAny, Pattern: ".*", Action: models.ActionKeep, Enabled: true}
			if err := store.SavePolicy(ctx, second); err != nil {
				t.Fatal(err)
			}
			if second.Priority != 6 {
				t.Fatalf("appended priority = %d", second.Priority)
			}
		})
	}
}

func TestMemoryClose(t *testing.T) {
	store := storage.NewMemory()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestGenjiInvalidPathAndReopen(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.NewGenji(filePath); err == nil {
		t.Fatal("expected error opening a regular file as a database")
	}

	dbPath := filepath.Join(dir, "reopen", "mailvoid.db")
	first, err := storage.NewGenji(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := first.SaveMessage(ctx, &models.Message{ID: "persisted", ReceivedAt: time.Now().UTC(), Mailbox: "inbox", Domain: "example.test"}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := storage.NewGenji(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := second.GetMessage(ctx, "persisted"); err != nil {
		t.Fatalf("reopened message: %v", err)
	}
}

func TestAPIKeyPersistence(t *testing.T) {
	ctx := context.Background()
	for name, store := range stores(t) {
		t.Run(name, func(t *testing.T) {
			expires := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)
			first := &models.APIKey{Name: "agent", Prefix: "prefix-a", TokenHash: "hash-a", CreatedAt: time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond), CreatedBy: "admin", ExpiresAt: &expires}
			second := &models.APIKey{Name: "other", Prefix: "prefix-b", TokenHash: "hash-b"}
			for _, key := range []*models.APIKey{first, second} {
				if err := store.SaveAPIKey(ctx, key); err != nil {
					t.Fatal(err)
				}
			}
			got, err := store.GetAPIKey(ctx, first.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != "agent" || got.Prefix != "prefix-a" || got.TokenHash != "hash-a" || got.CreatedBy != "admin" || got.ExpiresAt == nil {
				t.Fatalf("api key = %+v", got)
			}
			byPrefix, err := store.GetAPIKeyByPrefix(ctx, "prefix-a")
			if err != nil || byPrefix.ID != first.ID {
				t.Fatalf("by prefix = %+v err=%v", byPrefix, err)
			}
			list, err := store.ListAPIKeys(ctx)
			if err != nil || len(list) != 2 {
				t.Fatalf("list = %+v err=%v", list, err)
			}
			first.Name = "renamed"
			if err := store.SaveAPIKey(ctx, first); err != nil {
				t.Fatal(err)
			}
			got, _ = store.GetAPIKey(ctx, first.ID)
			if got.Name != "renamed" {
				t.Fatalf("update = %+v", got)
			}
			if err := store.DeleteAPIKey(ctx, first.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := store.GetAPIKey(ctx, first.ID); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("delete = %v", err)
			}
			if err := store.DeleteAPIKey(ctx, "missing"); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("delete missing = %v", err)
			}
			if _, err := store.GetAPIKeyByPrefix(ctx, "missing"); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("prefix missing = %v", err)
			}
		})
	}
}

func TestWebDAVUserPersistence(t *testing.T) {
	ctx := context.Background()
	for name, store := range stores(t) {
		t.Run(name, func(t *testing.T) {
			user := &models.WebDAVUser{Username: "DavUser", PasswordHash: "hash"}
			if err := store.SaveWebDAVUser(ctx, user); err != nil {
				t.Fatal(err)
			}
			got, err := store.GetWebDAVUser(ctx, "davuser")
			if err != nil || got.Username != "davuser" || got.PasswordHash != "hash" {
				t.Fatalf("get = %+v err=%v", got, err)
			}
			user.PasswordHash = "hash2"
			if err := store.SaveWebDAVUser(ctx, user); err != nil {
				t.Fatal(err)
			}
			got, _ = store.GetWebDAVUser(ctx, "davuser")
			if got.PasswordHash != "hash2" {
				t.Fatalf("update = %+v", got)
			}
			list, err := store.ListWebDAVUsers(ctx)
			if err != nil || len(list) != 1 {
				t.Fatalf("list = %+v err=%v", list, err)
			}
			if err := store.DeleteWebDAVUser(ctx, "DAVUSER"); err != nil {
				t.Fatal(err)
			}
			if _, err := store.GetWebDAVUser(ctx, "davuser"); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("delete = %v", err)
			}
			if err := store.DeleteWebDAVUser(ctx, "missing"); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("delete missing = %v", err)
			}
		})
	}
}

func TestSettingsPersistence(t *testing.T) {
	ctx := context.Background()
	for name, store := range stores(t) {
		t.Run(name, func(t *testing.T) {
			if _, err := store.GetSetting(ctx, "capture.enabled"); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("missing setting error = %v", err)
			}
			if err := store.SetSetting(ctx, "capture.enabled", "false"); err != nil {
				t.Fatal(err)
			}
			value, err := store.GetSetting(ctx, "capture.enabled")
			if err != nil {
				t.Fatal(err)
			}
			if value != "false" {
				t.Fatalf("value = %q", value)
			}
			if err := store.SetSetting(ctx, "capture.enabled", "true"); err != nil {
				t.Fatal(err)
			}
			value, _ = store.GetSetting(ctx, "capture.enabled")
			if value != "true" {
				t.Fatalf("updated value = %q", value)
			}
		})
	}
}
