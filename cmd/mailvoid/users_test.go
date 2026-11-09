package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/auth"
	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/storage"
)

func TestListUsers(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()
	created := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	for _, user := range []*models.User{
		{Username: "zeta", PasswordHash: "hash", CreatedAt: created.Add(time.Hour)},
		{Username: "admin", PasswordHash: "hash", CreatedAt: created},
	} {
		if err := store.SaveUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	if err := listUsers(ctx, store, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"USERNAME", "admin", "2026-11-08T12:00:00Z", "zeta"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
	if strings.Index(text, "admin") > strings.Index(text, "zeta") {
		t.Fatalf("users are not sorted:\n%s", text)
	}
}

func TestListUsersEmpty(t *testing.T) {
	var out bytes.Buffer
	if err := listUsers(context.Background(), storage.NewMemory(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No administrator users found") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestChangeUserPassword(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()
	oldHash, err := auth.HashPassword("old-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveUser(ctx, &models.User{Username: "admin", PasswordHash: oldHash}); err != nil {
		t.Fatal(err)
	}
	bobHash, err := auth.HashPassword("bob-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveUser(ctx, &models.User{Username: "bob", PasswordHash: bobHash}); err != nil {
		t.Fatal(err)
	}

	if err := changeUserPassword(ctx, store, " Admin ", "new-password"); err != nil {
		t.Fatal(err)
	}
	authSvc := auth.New(store)
	if _, err := authSvc.Authenticate(ctx, "admin", "new-password"); err != nil {
		t.Fatalf("new password rejected: %v", err)
	}
	if _, err := authSvc.Authenticate(ctx, "admin", "old-password"); err == nil {
		t.Fatal("old password still accepted")
	}
	if _, err := authSvc.Authenticate(ctx, "bob", "bob-password"); err != nil {
		t.Fatalf("unrelated user password changed: %v", err)
	}
}

func TestChangeUserPasswordValidation(t *testing.T) {
	store := storage.NewMemory()
	if err := changeUserPassword(context.Background(), store, "admin", "short"); err == nil {
		t.Fatal("expected short password error")
	}
	if err := changeUserPassword(context.Background(), store, "missing", "long-enough"); err == nil {
		t.Fatal("expected missing user error")
	}
}

func cliTestDB(t *testing.T, users map[string]string) (dir, dbPath string) {
	t.Helper()
	t.Setenv("MAILVOID_CONFIG", "")
	t.Setenv("MAILVOID_DATA_DIR", "")
	t.Setenv("MAILVOID_DB_PATH", "")
	t.Setenv("MAILVOID_ADMIN_USER", "")

	dir = t.TempDir()
	dbPath = filepath.Join(dir, "mailvoid.db")
	store, err := storage.NewGenji(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	for username, password := range users {
		hash, err := auth.HashPassword(password)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SaveUser(ctx, &models.User{Username: username, PasswordHash: hash, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return dir, dbPath
}

func withPasswdInput(t *testing.T, input string) {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	oldIn, oldErr := os.Stdin, os.Stderr
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = file
	os.Stderr = devNull
	t.Cleanup(func() {
		os.Stdin = oldIn
		os.Stderr = oldErr
		_ = file.Close()
		_ = devNull.Close()
	})
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	runErr := fn()
	_ = writer.Close()
	os.Stdout = old
	data, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(data), runErr
}

func cliArgs(dir, dbPath string) []string {
	return []string{"-config", "", "-data-dir", dir, "-db-path", dbPath}
}

func TestRunUsersListCLI(t *testing.T) {
	dir, dbPath := cliTestDB(t, map[string]string{"admin": "secret", "zeta": "secret"})
	out, err := captureStdout(t, func() error {
		return runUsersList(cliArgs(dir, dbPath))
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"USERNAME", "admin", "zeta", "2026-11-08T12:00:00Z"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunUsersPasswdCLI(t *testing.T) {
	dir, dbPath := cliTestDB(t, map[string]string{"admin": "old-password"})
	withPasswdInput(t, "new-password\nnew-password\n")
	out, err := captureStdout(t, func() error {
		return runUsersPasswd(append([]string{"admin"}, cliArgs(dir, dbPath)...))
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Password updated for admin") {
		t.Fatalf("output = %q", out)
	}

	store, err := storage.NewGenji(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	authSvc := auth.New(store)
	if _, err := authSvc.Authenticate(context.Background(), "admin", "new-password"); err != nil {
		t.Fatalf("new password rejected: %v", err)
	}
	if _, err := authSvc.Authenticate(context.Background(), "admin", "old-password"); err == nil {
		t.Fatal("old password still accepted")
	}
	user, err := store.GetUser(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if user.PasswordHash == "new-password" || !strings.HasPrefix(user.PasswordHash, "$2") {
		t.Fatalf("password was not hashed: %q", user.PasswordHash)
	}
}

func TestRunUsersPasswdDefaultUsername(t *testing.T) {
	dir, dbPath := cliTestDB(t, map[string]string{"admin": "old-password"})
	withPasswdInput(t, "new-password\nnew-password\n")
	if _, err := captureStdout(t, func() error {
		return runUsersPasswd(cliArgs(dir, dbPath))
	}); err != nil {
		t.Fatal(err)
	}

	store, err := storage.NewGenji(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if _, err := auth.New(store).Authenticate(context.Background(), "admin", "new-password"); err != nil {
		t.Fatalf("default admin password was not changed: %v", err)
	}
}

func TestRunUsersPasswdMismatch(t *testing.T) {
	dir, dbPath := cliTestDB(t, map[string]string{"admin": "old-password"})
	withPasswdInput(t, "new-password\nother-password\n")
	_, err := captureStdout(t, func() error {
		return runUsersPasswd(append([]string{"admin"}, cliArgs(dir, dbPath)...))
	})
	if err == nil || !strings.Contains(err.Error(), "do not match") {
		t.Fatalf("error = %v", err)
	}
	assertPasswordUnchanged(t, dbPath, "admin", "old-password")
}

func TestRunUsersPasswdShort(t *testing.T) {
	dir, dbPath := cliTestDB(t, map[string]string{"admin": "old-password"})
	withPasswdInput(t, "short\nshort\n")
	_, err := captureStdout(t, func() error {
		return runUsersPasswd(append([]string{"admin"}, cliArgs(dir, dbPath)...))
	})
	if err == nil || !strings.Contains(err.Error(), "at least 8") {
		t.Fatalf("error = %v", err)
	}
	assertPasswordUnchanged(t, dbPath, "admin", "old-password")
}

func TestRunUsersPasswdMissingUser(t *testing.T) {
	dir, dbPath := cliTestDB(t, map[string]string{"admin": "old-password"})
	withPasswdInput(t, "new-password\nnew-password\n")
	_, err := captureStdout(t, func() error {
		return runUsersPasswd(append([]string{"missing"}, cliArgs(dir, dbPath)...))
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunUsersPasswdEmptyInput(t *testing.T) {
	dir, dbPath := cliTestDB(t, map[string]string{"admin": "old-password"})
	withPasswdInput(t, "")
	_, err := captureStdout(t, func() error {
		return runUsersPasswd(append([]string{"admin"}, cliArgs(dir, dbPath)...))
	})
	if err == nil || !strings.Contains(err.Error(), "at least 8") {
		t.Fatalf("error = %v", err)
	}
}

func assertPasswordUnchanged(t *testing.T, dbPath, username, password string) {
	t.Helper()
	store, err := storage.NewGenji(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if _, err := auth.New(store).Authenticate(context.Background(), username, password); err != nil {
		t.Fatalf("original password rejected after failed change: %v", err)
	}
}

func TestRunUserCommand(t *testing.T) {
	if handled, err := runUserCommand(nil); handled || err != nil {
		t.Fatalf("nil args = %v, %v", handled, err)
	}
	if handled, err := runUserCommand([]string{"-smtp-port", "1025"}); handled || err != nil {
		t.Fatalf("server flag = %v, %v", handled, err)
	}
	out, err := captureStdout(t, func() error {
		_, runErr := runUserCommand([]string{"users", "help"})
		return runErr
	})
	if err != nil || !strings.Contains(out, "users passwd") {
		t.Fatalf("users help output = %q, %v", out, err)
	}
	if handled, err := runUserCommand([]string{"users", "bogus"}); !handled || err == nil {
		t.Fatalf("bogus users command = %v, %v", handled, err)
	}
}
