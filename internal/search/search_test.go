package search_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/search"
	"github.com/gnrfan/mailvoid/internal/storage"
)

func seedSearch(t *testing.T, store storage.Store) (time.Time, map[string]*models.Message) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	messages := map[string]*models.Message{
		"m1": {ID: "m1", ReceivedAt: now.Add(-3 * time.Minute), Mailbox: "inbox", Domain: "example.test", Recipient: "inbox@example.test", Sender: "alice@example.org", Subject: "Welcome invoice", BodyText: "Your invoice is attached.", BodyHTML: "<p>HTML invoice body</p>", RawHeaders: "From: alice@example.org\r\nX-Test: one\r\n", RemoteIP: "10.0.0.1", LocalPort: 1025, Action: models.ActionKeep, Size: 42},
		"m2": {ID: "m2", ReceivedAt: now.Add(-2 * time.Minute), Mailbox: "other", Domain: "other.test", Recipient: "other@other.test", Sender: "bob@example.org", Subject: "Newsletter", BodyText: "Monthly updates and news.", BodyHTML: "", RemoteIP: "10.0.0.2", LocalPort: 2525, Action: models.ActionTTL, Size: 11},
		"m3": {ID: "m3", ReceivedAt: now.Add(-1 * time.Minute), Mailbox: "inbox", Domain: "example.test", Recipient: "inbox@example.test", Sender: "carol@example.org", Subject: "Alert", BodyText: "Urgent invoice reminder.", RemoteIP: "10.0.0.3", LocalPort: 1025, Action: models.ActionKeep, Size: 23},
	}
	for _, message := range messages {
		if err := store.SaveMessage(context.Background(), message); err != nil {
			t.Fatal(err)
		}
	}
	return now, messages
}

func openSearch(t *testing.T, store storage.Store) *search.Bleve {
	t.Helper()
	index, created, err := search.Open(filepath.Join(t.TempDir(), "search.bleve"))
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected a new index")
	}
	if err := index.Ensure(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	return index
}

func TestFullTextSearch(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()
	now, _ := seedSearch(t, store)
	index := openSearch(t, store)

	results, err := index.Search(ctx, search.Query{Text: "invoice"})
	if err != nil {
		t.Fatal(err)
	}
	if results.Total != 2 || len(results.Hits) != 2 {
		t.Fatalf("invoice results = %+v", results)
	}
	if len(results.Hits[0].Highlights) == 0 {
		t.Fatalf("expected highlights: %+v", results.Hits[0])
	}

	results, err = index.Search(ctx, search.Query{Text: "monthly"})
	if err != nil || results.Total != 1 || results.Hits[0].ID != "m2" {
		t.Fatalf("monthly = %+v err=%v", results, err)
	}

	// HTML is stripped before indexing.
	results, _ = index.Search(ctx, search.Query{Text: "HTML"})
	if results.Total != 1 || results.Hits[0].ID != "m1" {
		t.Fatalf("html = %+v", results)
	}

	results, _ = index.Search(ctx, search.Query{Text: "invoice", Mailbox: "inbox", Domain: "example.test"})
	if results.Total != 2 {
		t.Fatalf("mailbox filter = %+v", results)
	}
	results, _ = index.Search(ctx, search.Query{Text: "invoice", Port: 1025, Action: models.ActionKeep})
	if results.Total != 2 {
		t.Fatalf("port/action filter = %+v", results)
	}
	results, _ = index.Search(ctx, search.Query{Text: "invoice", Since: now.Add(-90 * time.Second)})
	if results.Total != 1 || results.Hits[0].ID != "m3" {
		t.Fatalf("since = %+v", results)
	}
	results, _ = index.Search(ctx, search.Query{Text: "invoice", Until: now.Add(-90 * time.Second)})
	if results.Total != 1 || results.Hits[0].ID != "m1" {
		t.Fatalf("until = %+v", results)
	}
	results, _ = index.Search(ctx, search.Query{Text: "invoice", Sender: "carol@example.org"})
	if results.Total != 1 || results.Hits[0].ID != "m3" {
		t.Fatalf("sender = %+v", results)
	}
	results, _ = index.Search(ctx, search.Query{Text: "invoice", Recipient: "inbox@example.test"})
	if results.Total != 2 {
		t.Fatalf("recipient = %+v", results)
	}

	results, _ = index.Search(ctx, search.Query{Text: "invoice", Limit: 1, Offset: 1, Sort: "received_at"})
	if results.Total != 2 || len(results.Hits) != 1 || results.Hits[0].ID != "m1" {
		t.Fatalf("sort/pagination = %+v", results)
	}

	results, _ = index.Search(ctx, search.Query{})
	if results.Total != 3 {
		t.Fatalf("match all = %+v", results)
	}
}

func TestIndexedStoreAndRebuild(t *testing.T) {
	ctx := context.Background()
	base := storage.NewMemory()
	index, _, err := search.Open(filepath.Join(t.TempDir(), "search.bleve"))
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	store := search.NewIndexedStore(base, index, nil)

	now := time.Now().UTC().Truncate(time.Second)
	first := &models.Message{ID: "s1", ReceivedAt: now, Mailbox: "inbox", Domain: "example.test", Recipient: "inbox@example.test", Sender: "a@b.test", Subject: "Saved", BodyText: "unique searchable token", Action: models.ActionKeep}
	if err := store.SaveMessage(ctx, first); err != nil {
		t.Fatal(err)
	}
	results, err := index.Search(ctx, search.Query{Text: "searchable"})
	if err != nil || results.Total != 1 || results.Hits[0].ID != "s1" {
		t.Fatalf("indexed save = %+v err=%v", results, err)
	}

	if err := store.DeleteMessage(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	results, _ = index.Search(ctx, search.Query{Text: "searchable"})
	if results.Total != 0 {
		t.Fatalf("indexed delete = %+v", results)
	}

	expired := &models.Message{ID: "s2", ReceivedAt: now.Add(-time.Hour), Mailbox: "inbox", Domain: "example.test", Recipient: "inbox@example.test", Subject: "Expired", BodyText: "expired token", Action: models.ActionTTL, ExpiresAt: timePtr(now.Add(-time.Minute))}
	if err := store.SaveMessage(ctx, expired); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeleteExpired(ctx, now); err != nil {
		t.Fatal(err)
	}
	results, _ = index.Search(ctx, search.Query{Text: "expired"})
	if results.Total != 0 {
		t.Fatalf("expired index = %+v", results)
	}

	// Bypass the index, then Ensure should notice the count mismatch and rebuild.
	direct := &models.Message{ID: "s3", ReceivedAt: now, Mailbox: "box", Domain: "example.test", Recipient: "box@example.test", Subject: "Direct", BodyText: "rebuild token", Action: models.ActionKeep}
	if err := base.SaveMessage(ctx, direct); err != nil {
		t.Fatal(err)
	}
	if err := index.Ensure(ctx, base); err != nil {
		t.Fatal(err)
	}
	results, _ = index.Search(ctx, search.Query{Text: "rebuild"})
	if results.Total != 1 || results.Hits[0].ID != "s3" {
		t.Fatalf("rebuild = %+v", results)
	}
}

func TestReopenAndDeletedIndexing(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()
	seedSearch(t, store)
	path := filepath.Join(t.TempDir(), "index")
	first, created, err := search.Open(path)
	if err != nil || !created {
		t.Fatalf("open = %v created=%v", err, created)
	}
	if err := first.Ensure(ctx, store); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, created, err := search.Open(path)
	if err != nil || created {
		t.Fatalf("reopen = %v created=%v", err, created)
	}
	defer second.Close()
	results, err := second.Search(ctx, search.Query{Text: "invoice"})
	if err != nil || results.Total != 2 {
		t.Fatalf("reopened search = %+v err=%v", results, err)
	}
	message := &models.Message{ID: "m3", Deleted: true}
	if err := second.IndexMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	results, _ = second.Search(ctx, search.Query{Text: "invoice"})
	if results.Total != 1 {
		t.Fatalf("deleted indexing = %+v", results)
	}
}

func TestClosedIndex(t *testing.T) {
	index, _, err := search.Open(filepath.Join(t.TempDir(), "closed"))
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Close(); err != nil {
		t.Fatal(err)
	}
	if err := index.IndexMessage(context.Background(), &models.Message{ID: "x"}); err == nil {
		t.Fatal("expected index error")
	}
	if _, err := index.Search(context.Background(), search.Query{Text: "x"}); err == nil {
		t.Fatal("expected search error")
	}
	if err := index.DeleteMessage(context.Background(), "x"); err == nil {
		t.Fatal("expected delete error")
	}
}

func timePtr(t time.Time) *time.Time { return &t }
