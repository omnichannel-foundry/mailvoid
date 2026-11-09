// Package auth provides bcrypt credential storage and session-independent
// authentication for MailVoid administrators.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials is returned for any failed login attempt.
var ErrInvalidCredentials = errors.New("invalid username or password")

// ErrInvalidCredential is returned when a managed credential cannot be created.
var ErrInvalidCredential = errors.New("invalid credential")

const (
	bcryptCost             = 12
	webDAVCacheTTL         = 5 * time.Minute
	webDAVPasswordMinChars = 12
)

// A pre-computed hash used to keep failed-login timing roughly constant when
// the username does not exist.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("mailvoid-dummy-password"), bcryptCost)

// Service authenticates users and manages API and WebDAV credentials.
type Service struct {
	store       storage.Store
	webdavMu    sync.Mutex
	webdavCache map[string]time.Time
}

// New creates an auth service.
func New(store storage.Store) *Service {
	return &Service{store: store, webdavCache: make(map[string]time.Time)}
}

// HashPassword returns a bcrypt hash.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// Authenticate verifies a username/password pair.
func (s *Service) Authenticate(ctx context.Context, username, password string) (*models.User, error) {
	user, err := s.store.GetUser(ctx, strings.ToLower(strings.TrimSpace(username)))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return user, nil
}

// BootstrapResult describes what EnsureBootstrap did.
type BootstrapResult struct {
	Created        bool
	Updated        bool
	GeneratedPass  string
	Username       string
	PasswordInHash bool
}

// EnsureBootstrap guarantees an administrator exists. When password is empty a
// random password is generated and returned. An existing user is only updated
// when reset is true.
func (s *Service) EnsureBootstrap(ctx context.Context, username, password string, reset bool) (BootstrapResult, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		username = "admin"
	}

	existing, err := s.store.GetUser(ctx, username)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return BootstrapResult{}, err
	}

	result := BootstrapResult{Username: username}
	if existing != nil && !reset {
		return result, nil
	}

	plain := password
	if plain == "" {
		generated, err := randomPassword()
		if err != nil {
			return BootstrapResult{}, err
		}
		plain = generated
		result.GeneratedPass = generated
	}
	hash, err := HashPassword(plain)
	if err != nil {
		return BootstrapResult{}, err
	}
	user := &models.User{
		Username:     username,
		PasswordHash: hash,
		CreatedAt:    time.Now().UTC(),
	}
	if existing != nil {
		user.CreatedAt = existing.CreatedAt
	}
	if err := s.store.SaveUser(ctx, user); err != nil {
		return BootstrapResult{}, err
	}
	if existing == nil {
		result.Created = true
	} else {
		result.Updated = true
	}
	return result, nil
}

// SetPassword hashes and stores a new password for an existing user.
func (s *Service) SetPassword(ctx context.Context, username, password string) error {
	user, err := s.store.GetUser(ctx, username)
	if err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	user.PasswordHash = hash
	return s.store.SaveUser(ctx, user)
}

// CreateAPIKey generates a token, stores only its hash, and returns the
// plaintext token exactly once.
func (s *Service) CreateAPIKey(ctx context.Context, name string, expiresAt *time.Time, createdBy string) (string, *models.APIKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, ErrInvalidCredential
	}
	if expiresAt != nil && !expiresAt.After(time.Now().UTC()) {
		return "", nil, ErrInvalidCredential
	}

	for attempt := 0; attempt < 5; attempt++ {
		token, prefix, hash, err := generateAPIKeyToken()
		if err != nil {
			return "", nil, err
		}
		if _, err := s.store.GetAPIKeyByPrefix(ctx, prefix); err == nil {
			continue
		} else if !errors.Is(err, storage.ErrNotFound) {
			return "", nil, err
		}
		key := &models.APIKey{
			ID:        models.NewID("key"),
			Name:      name,
			Prefix:    prefix,
			TokenHash: hash,
			CreatedAt: time.Now().UTC(),
			ExpiresAt: expiresAt,
			CreatedBy: createdBy,
		}
		if err := s.store.SaveAPIKey(ctx, key); err != nil {
			return "", nil, err
		}
		return token, key, nil
	}
	return "", nil, errors.New("could not allocate a unique api key prefix")
}

// AuthenticateAPIKey validates a managed bearer token.
func (s *Service) AuthenticateAPIKey(ctx context.Context, token string) (*models.APIKey, error) {
	parts := strings.SplitN(strings.TrimSpace(token), "_", 3)
	if len(parts) != 3 || parts[0] != "mv" || parts[1] == "" || parts[2] == "" {
		return nil, ErrInvalidCredentials
	}
	key, err := s.store.GetAPIKeyByPrefix(ctx, parts[1])
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	sum := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(key.TokenHash)) != 1 {
		return nil, ErrInvalidCredentials
	}
	if key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now().UTC()) {
		return nil, ErrInvalidCredentials
	}
	return key, nil
}

// CreateWebDAVUser creates a managed Basic-auth credential. If password is
// empty, a strong password is generated and returned once.
func (s *Service) CreateWebDAVUser(ctx context.Context, username, password string) (string, *models.WebDAVUser, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" || strings.ContainsAny(username, ": \t\r\n") {
		return "", nil, ErrInvalidCredential
	}
	generated := ""
	if password == "" {
		var err error
		password, err = randomPassword()
		if err != nil {
			return "", nil, err
		}
		generated = password
	} else if len(password) < webDAVPasswordMinChars {
		return "", nil, ErrInvalidCredential
	}
	hash, err := HashPassword(password)
	if err != nil {
		return "", nil, err
	}
	user := &models.WebDAVUser{
		Username:     username,
		PasswordHash: hash,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.store.SaveWebDAVUser(ctx, user); err != nil {
		return "", nil, err
	}
	s.InvalidateWebDAVCache()
	return generated, user, nil
}

// VerifyWebDAV checks a managed WebDAV username/password pair. Successful
// checks are cached briefly so a WebDAV client's many requests do not pay the
// bcrypt cost each time.
func (s *Service) VerifyWebDAV(ctx context.Context, username, password string) error {
	username = strings.ToLower(strings.TrimSpace(username))
	cacheKey := webDAVCacheKey(username, password)

	s.webdavMu.Lock()
	expires, ok := s.webdavCache[cacheKey]
	if ok && time.Now().Before(expires) {
		s.webdavMu.Unlock()
		return nil
	}
	if ok {
		delete(s.webdavCache, cacheKey)
	}
	s.webdavMu.Unlock()

	user, err := s.store.GetWebDAVUser(ctx, username)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return ErrInvalidCredentials
		}
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return ErrInvalidCredentials
	}

	s.webdavMu.Lock()
	s.webdavCache[cacheKey] = time.Now().Add(webDAVCacheTTL)
	s.webdavMu.Unlock()
	return nil
}

// InvalidateWebDAVCache clears cached Basic-auth successes after credential
// changes.
func (s *Service) InvalidateWebDAVCache() {
	s.webdavMu.Lock()
	s.webdavCache = make(map[string]time.Time)
	s.webdavMu.Unlock()
}

func webDAVCacheKey(username, password string) string {
	sum := sha256.Sum256([]byte(password))
	return username + "\x00" + hex.EncodeToString(sum[:])
}

func generateAPIKeyToken() (token, prefix, hash string, err error) {
	prefixBytes := make([]byte, 6)
	if _, err := rand.Read(prefixBytes); err != nil {
		return "", "", "", err
	}
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", "", "", err
	}
	prefix = hex.EncodeToString(prefixBytes)
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	token = "mv_" + prefix + "_" + secret
	sum := sha256.Sum256([]byte(token))
	return token, prefix, hex.EncodeToString(sum[:]), nil
}

func randomPassword() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
