package web

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/mailbox"
	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/storage"
)

func davFixture(t *testing.T) (*davFS, storage.Store) {
	t.Helper()
	store := storage.NewMemory()
	now := time.Now().UTC()
	for _, message := range []*models.Message{
		{ID: "msg_inbox", Mailbox: "inbox", Domain: "example.test", Recipient: "inbox@example.test", Subject: "Hello", Raw: "raw-inbox", ReceivedAt: now, Action: models.ActionKeep},
		{ID: "msg_other", Mailbox: "other", Domain: "example.test", Recipient: "other@example.test", Subject: "Other", Raw: "raw-other", ReceivedAt: now.Add(-time.Minute), Action: models.ActionKeep},
	} {
		if err := store.SaveMessage(context.Background(), message); err != nil {
			t.Fatal(err)
		}
	}
	return &davFS{store: store, mailboxes: mailbox.New(store)}, store
}

func TestDAVReadOnlyOperations(t *testing.T) {
	fs, _ := davFixture(t)
	ctx := context.Background()
	if err := fs.Mkdir(ctx, "/x", 0o755); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("mkdir = %v", err)
	}
	if err := fs.RemoveAll(ctx, "/x"); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("remove = %v", err)
	}
	if err := fs.Rename(ctx, "/a", "/b"); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("rename = %v", err)
	}
	if _, err := fs.OpenFile(ctx, "/", os.O_WRONLY, 0); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("write open = %v", err)
	}
	if _, err := fs.OpenFile(ctx, "/inbox@example.test/msg_inbox.eml", os.O_RDWR, 0); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("rw open = %v", err)
	}
}

func TestDAVRootAndMailboxListing(t *testing.T) {
	fs, _ := davFixture(t)
	ctx := context.Background()

	root, err := fs.OpenFile(ctx, "/", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	rootInfo, err := root.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if !rootInfo.IsDir() {
		t.Fatal("root should be a directory")
	}
	entries, err := root.Readdir(-1)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("root entries = %d", len(entries))
	}

	dir, err := fs.OpenFile(ctx, "/inbox@example.test", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	entries, err = dir.Readdir(-1)
	if err != nil || len(entries) != 1 || entries[0].Name() != "msg_inbox.eml" {
		t.Fatalf("mailbox entries = %+v err=%v", entries, err)
	}

	// Paginated Readdir semantics.
	dir2, _ := fs.OpenFile(ctx, "/", os.O_RDONLY, 0)
	first, err := dir2.Readdir(1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first readdir = %+v err=%v", first, err)
	}
	second, err := dir2.Readdir(10)
	if err != nil || len(second) != 1 {
		t.Fatalf("second readdir = %+v err=%v", second, err)
	}
	if _, err := dir2.Readdir(10); !errors.Is(err, io.EOF) {
		t.Fatalf("end readdir = %v", err)
	}

	if _, err := fs.OpenFile(ctx, "/missing@example.test", os.O_RDONLY, 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing mailbox = %v", err)
	}
	if _, err := fs.OpenFile(ctx, "/a/b/c", os.O_RDONLY, 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deep path = %v", err)
	}
}

func TestDAVFileReadAndStat(t *testing.T) {
	fs, _ := davFixture(t)
	ctx := context.Background()

	file, err := fs.OpenFile(ctx, "/inbox@example.test/msg_inbox.eml", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	if err != nil || string(data) != "raw-inbox" {
		t.Fatalf("read = %q err=%v", data, err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	first := make([]byte, 3)
	if n, err := file.Read(first); err != nil || n != 3 || string(first) != "raw" {
		t.Fatalf("read after seek = %q n=%d err=%v", first, n, err)
	}
	info, err := file.Stat()
	if err != nil || info.Size() != int64(len("raw-inbox")) || info.IsDir() {
		t.Fatalf("info = %+v err=%v", info, err)
	}
	if _, err := file.Write([]byte("x")); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("write = %v", err)
	}

	dir, _ := fs.OpenFile(ctx, "/", os.O_RDONLY, 0)
	if _, err := dir.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("dir read = %v", err)
	}
	if _, err := dir.Seek(0, io.SeekStart); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("dir seek = %v", err)
	}
	if _, err := dir.Readdir(1); err != nil {
		t.Fatal(err)
	}

	stat, err := fs.Stat(ctx, "/inbox@example.test/msg_inbox.eml")
	if err != nil || stat.Name() != "msg_inbox.eml" {
		t.Fatalf("stat = %+v err=%v", stat, err)
	}
	if _, err := fs.Stat(ctx, "/inbox@example.test/missing.eml"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing stat = %v", err)
	}
}

func TestDAVCrossMailboxAndExtensionChecks(t *testing.T) {
	fs, _ := davFixture(t)
	ctx := context.Background()
	if _, err := fs.OpenFile(ctx, "/other@example.test/msg_inbox.eml", os.O_RDONLY, 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cross mailbox = %v", err)
	}
	if _, err := fs.OpenFile(ctx, "/inbox@example.test/msg_inbox.txt", os.O_RDONLY, 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrong extension = %v", err)
	}
	if _, err := fs.OpenFile(ctx, "/inbox@example.test/../other@example.test/msg_other.eml", os.O_RDONLY, 0); err != nil {
		t.Fatalf("clean path = %v", err)
	}
}

func TestEMLFilename(t *testing.T) {
	cases := []struct {
		subject string
		id      string
		want    string
	}{
		{"Hello World", "msg_abc", "hello-world-abc.eml"},
		{"", "msg_abc", "message-abc.eml"},
		{"../weird\r\nname", "msg_x", "weird-name-x.eml"},
		{"  Invoice #42  ", "msg_y", "invoice-42-y.eml"},
	}
	for _, tc := range cases {
		got := emlFilename(&models.Message{Subject: tc.subject, ID: tc.id})
		if got != tc.want {
			t.Errorf("emlFilename(%q) = %q, want %q", tc.subject, got, tc.want)
		}
		if strings.ContainsAny(got, "\r\n/\\") {
			t.Errorf("unsafe filename %q", got)
		}
	}
	long := strings.Repeat("a", 200)
	got := emlFilename(&models.Message{Subject: long, ID: "msg_z"})
	if len(got) > 80 || !strings.HasSuffix(got, ".eml") {
		t.Fatalf("long filename = %q", got)
	}
}

func TestDAVFileInfoModeAndSys(t *testing.T) {
	file := davFileInfo{name: "x.eml", size: 5, mode: 0o644}
	if file.Mode()&os.ModeDir != 0 || file.Mode()&0o644 == 0 {
		t.Fatalf("file mode = %v", file.Mode())
	}
	dir := davFileInfo{name: "dir", dir: true}
	if !dir.IsDir() || dir.Mode()&os.ModeDir == 0 {
		t.Fatalf("dir mode = %v", dir.Mode())
	}
	if file.Sys() != nil || file.Name() != "x.eml" || file.Size() != 5 {
		t.Fatal("file info mismatch")
	}
	if !latestModTime([]os.FileInfo{
		davFileInfo{modTime: time.Unix(10, 0)},
		davFileInfo{modTime: time.Unix(20, 0)},
	}).Equal(time.Unix(20, 0)) {
		t.Fatal("latest mod time")
	}
}
