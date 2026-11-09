package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gnrfan/mailvoid/internal/auth"
	"github.com/gnrfan/mailvoid/internal/storage"
)

func TestHashAndAuthenticate(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()
	service := auth.New(store)

	result, err := service.EnsureBootstrap(ctx, "Admin", "secret123", false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.Username != "admin" {
		t.Fatalf("result = %+v", result)
	}

	user, err := service.Authenticate(ctx, "ADMIN", "secret123")
	if err != nil {
		t.Fatal(err)
	}
	if user.Username != "admin" {
		t.Fatalf("user = %+v", user)
	}
	if user.PasswordHash == "secret123" {
		t.Fatal("password stored in plaintext")
	}

	if _, err := service.Authenticate(ctx, "admin", "wrong"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v", err)
	}
	if _, err := service.Authenticate(ctx, "missing", "secret123"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("missing user error = %v", err)
	}
}

func TestEnsureBootstrapDoesNotOverwrite(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()
	service := auth.New(store)

	if _, err := service.EnsureBootstrap(ctx, "admin", "first", false); err != nil {
		t.Fatal(err)
	}
	result, err := service.EnsureBootstrap(ctx, "admin", "second", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created || result.Updated {
		t.Fatalf("result = %+v", result)
	}
	if _, err := service.Authenticate(ctx, "admin", "first"); err != nil {
		t.Fatalf("original password should still work: %v", err)
	}

	result, err = service.EnsureBootstrap(ctx, "admin", "second", true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated {
		t.Fatalf("result = %+v", result)
	}
	if _, err := service.Authenticate(ctx, "admin", "second"); err != nil {
		t.Fatalf("new password should work: %v", err)
	}
}

func TestGeneratedPassword(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()
	service := auth.New(store)

	result, err := service.EnsureBootstrap(ctx, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.GeneratedPass == "" || len(result.GeneratedPass) < 12 {
		t.Fatalf("generated = %q", result.GeneratedPass)
	}
	if _, err := service.Authenticate(ctx, result.Username, result.GeneratedPass); err != nil {
		t.Fatal(err)
	}
}

func TestSetPassword(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()
	service := auth.New(store)
	if _, err := service.EnsureBootstrap(ctx, "admin", "old-password", false); err != nil {
		t.Fatal(err)
	}
	if err := service.SetPassword(ctx, "admin", "new-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, "admin", "new-password"); err != nil {
		t.Fatalf("new password: %v", err)
	}
	if _, err := service.Authenticate(ctx, "admin", "old-password"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("old password should fail: %v", err)
	}
	if err := service.SetPassword(ctx, "missing", "password"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("missing user: %v", err)
	}
}

func TestBootstrapResetGeneratesPassword(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()
	service := auth.New(store)
	if _, err := service.EnsureBootstrap(ctx, "admin", "first", false); err != nil {
		t.Fatal(err)
	}
	result, err := service.EnsureBootstrap(ctx, "admin", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.GeneratedPass == "" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := service.Authenticate(ctx, "admin", result.GeneratedPass); err != nil {
		t.Fatalf("generated password: %v", err)
	}
}

func TestAPIKeys(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()
	service := auth.New(store)

	if _, _, err := service.CreateAPIKey(ctx, "", nil, "admin"); !errors.Is(err, auth.ErrInvalidCredential) {
		t.Fatalf("empty name = %v", err)
	}
	past := time.Now().Add(-time.Hour)
	if _, _, err := service.CreateAPIKey(ctx, "expired", &past, "admin"); !errors.Is(err, auth.ErrInvalidCredential) {
		t.Fatalf("expired = %v", err)
	}

	token, key, err := service.CreateAPIKey(ctx, "agent", nil, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, "mv_") || key.ID == "" || key.Prefix == "" || key.TokenHash == "" || key.CreatedBy != "admin" {
		t.Fatalf("token=%q key=%+v", token, key)
	}
	if strings.Contains(key.TokenHash, token) {
		t.Fatal("token hash must not contain plaintext token")
	}
	got, err := service.AuthenticateAPIKey(ctx, token)
	if err != nil || got.ID != key.ID {
		t.Fatalf("authenticate = %+v err=%v", got, err)
	}
	for _, badToken := range []string{"", "nope", "mv_only", "mv__secret", "mv_prefix_", "mv_" + key.Prefix + "_wrong"} {
		if _, err := service.AuthenticateAPIKey(ctx, badToken); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Errorf("token %q err = %v", badToken, err)
		}
	}

	future := time.Now().Add(-time.Minute)
	key.ExpiresAt = &future
	if err := store.SaveAPIKey(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateAPIKey(ctx, token); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expired auth = %v", err)
	}
}

func TestWebDAVUsers(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemory()
	service := auth.New(store)

	if _, _, err := service.CreateWebDAVUser(ctx, "", "long-enough-password"); !errors.Is(err, auth.ErrInvalidCredential) {
		t.Fatalf("empty username = %v", err)
	}
	if _, _, err := service.CreateWebDAVUser(ctx, "bad:user", "long-enough-password"); !errors.Is(err, auth.ErrInvalidCredential) {
		t.Fatalf("colon username = %v", err)
	}
	if _, _, err := service.CreateWebDAVUser(ctx, "dav", "short"); !errors.Is(err, auth.ErrInvalidCredential) {
		t.Fatalf("short password = %v", err)
	}

	generated, user, err := service.CreateWebDAVUser(ctx, "DavUser", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) < 12 || user.Username != "davuser" || user.PasswordHash == generated {
		t.Fatalf("generated=%q user=%+v", generated, user)
	}
	if err := service.VerifyWebDAV(ctx, "davuser", generated); err != nil {
		t.Fatalf("verify = %v", err)
	}
	if err := service.VerifyWebDAV(ctx, "davuser", "wrong-password"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("wrong = %v", err)
	}
	if err := service.VerifyWebDAV(ctx, "missing", "password"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("missing = %v", err)
	}

	if _, _, err := service.CreateWebDAVUser(ctx, "davuser", "another-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := service.VerifyWebDAV(ctx, "davuser", "another-long-password"); err != nil {
		t.Fatalf("rotated password = %v", err)
	}
	if err := store.DeleteWebDAVUser(ctx, "davuser"); err != nil {
		t.Fatal(err)
	}
	// The cache would otherwise keep the old credential valid.
	if err := service.VerifyWebDAV(ctx, "davuser", "another-long-password"); err != nil {
		t.Fatalf("cached credential should still pass before invalidation: %v", err)
	}
	service.InvalidateWebDAVCache()
	if err := service.VerifyWebDAV(ctx, "davuser", "another-long-password"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("after invalidation = %v", err)
	}
}
