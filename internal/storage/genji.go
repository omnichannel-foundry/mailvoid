package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/genjidb/genji"
	"github.com/genjidb/genji/document"
	"github.com/genjidb/genji/types"
	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/models"
)

const messageColumns = `id, received_at, remote_ip, local_port, domain, mailbox, recipient,
	sender, subject, headers, raw_headers, raw, body_text, body_html, size_bytes,
	policy_id, policy_name, policy_action, expires_at, deleted, deleted_at`

const eventColumns = `id, event_time, event_type, severity, payload`

// Genji is a Store backed by the embedded Genji document database.
type Genji struct {
	mu sync.RWMutex
	db *genji.DB
}

type dbMessage struct {
	ID         string `genji:"id"`
	ReceivedAt int64  `genji:"received_at"`
	RemoteIP   string `genji:"remote_ip"`
	LocalPort  int    `genji:"local_port"`
	Domain     string `genji:"domain"`
	Mailbox    string `genji:"mailbox"`
	Recipient  string `genji:"recipient"`
	Sender     string `genji:"sender"`
	Subject    string `genji:"subject"`
	Headers    string `genji:"headers"`
	RawHeaders string `genji:"raw_headers"`
	Raw        string `genji:"raw"`
	BodyText   string `genji:"body_text"`
	BodyHTML   string `genji:"body_html"`
	Size       int64  `genji:"size_bytes"`
	PolicyID   string `genji:"policy_id"`
	PolicyName string `genji:"policy_name"`
	Action     string `genji:"policy_action"`
	ExpiresAt  int64  `genji:"expires_at"`
	Deleted    int    `genji:"deleted"`
	DeletedAt  int64  `genji:"deleted_at"`
}

type dbPolicy struct {
	ID         string `genji:"id"`
	Name       string `genji:"name"`
	Field      string `genji:"field_name"`
	Pattern    string `genji:"pattern"`
	Action     string `genji:"policy_action"`
	TTLSeconds int64  `genji:"ttl_seconds"`
	Priority   int    `genji:"priority"`
	Enabled    int    `genji:"enabled"`
	CreatedAt  int64  `genji:"created_at"`
	UpdatedAt  int64  `genji:"updated_at"`
}

type dbEvent struct {
	ID        string `genji:"id"`
	Timestamp int64  `genji:"event_time"`
	Type      string `genji:"event_type"`
	Severity  string `genji:"severity"`
	Payload   string `genji:"payload"`
}

type dbUser struct {
	Username     string `genji:"username"`
	PasswordHash string `genji:"password_hash"`
	CreatedAt    int64  `genji:"created_at"`
}

type dbAPIKey struct {
	ID        string `genji:"id"`
	Name      string `genji:"name"`
	Prefix    string `genji:"prefix"`
	TokenHash string `genji:"token_hash"`
	CreatedAt int64  `genji:"created_at"`
	ExpiresAt int64  `genji:"expires_at"`
	CreatedBy string `genji:"created_by"`
}

type dbWebDAVUser struct {
	Username     string `genji:"username"`
	PasswordHash string `genji:"password_hash"`
	CreatedAt    int64  `genji:"created_at"`
}

// NewGenji opens (and initializes) the database at path.
func NewGenji(path string) (*Genji, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := genji.Open(path)
	if err != nil {
		return nil, err
	}

	s := &Genji{db: db}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			received_at INT,
			remote_ip TEXT,
			local_port INT,
			domain TEXT,
			mailbox TEXT,
			recipient TEXT,
			sender TEXT,
			subject TEXT,
			headers TEXT,
			raw_headers TEXT,
			raw TEXT,
			body_text TEXT,
			body_html TEXT,
			size_bytes INT,
			policy_id TEXT,
			policy_name TEXT,
			policy_action TEXT,
			expires_at INT,
			deleted INT,
			deleted_at INT,
			search_text TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS policies (
			id TEXT PRIMARY KEY,
			name TEXT,
			field_name TEXT,
			pattern TEXT,
			policy_action TEXT,
			ttl_seconds INT,
			priority INT,
			enabled INT,
			created_at INT,
			updated_at INT
		)`,
		`CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			event_time INT,
			event_type TEXT,
			severity TEXT,
			payload TEXT,
			search_text TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			username TEXT PRIMARY KEY,
			password_hash TEXT,
			created_at INT
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			setting_key TEXT PRIMARY KEY,
			setting_value TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			name TEXT,
			prefix TEXT,
			token_hash TEXT,
			created_at INT,
			expires_at INT,
			created_by TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS webdav_users (
			username TEXT PRIMARY KEY,
			password_hash TEXT,
			created_at INT
		)`,
	} {
		if err := db.Exec(stmt); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("initialize schema: %w", err)
		}
	}
	return s, nil
}

func millis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func timeFromMillis(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

func timePtrFromMillis(ms int64) *time.Time {
	if ms == 0 {
		return nil
	}
	t := timeFromMillis(ms)
	return &t
}

func messageToDB(m *models.Message) dbMessage {
	headers, _ := json.Marshal(m.Headers)
	return dbMessage{
		ID:         m.ID,
		ReceivedAt: millis(m.ReceivedAt),
		RemoteIP:   m.RemoteIP,
		LocalPort:  m.LocalPort,
		Domain:     m.Domain,
		Mailbox:    m.Mailbox,
		Recipient:  m.Recipient,
		Sender:     m.Sender,
		Subject:    m.Subject,
		Headers:    string(headers),
		RawHeaders: m.RawHeaders,
		Raw:        m.Raw,
		BodyText:   m.BodyText,
		BodyHTML:   m.BodyHTML,
		Size:       m.Size,
		PolicyID:   m.PolicyID,
		PolicyName: m.PolicyName,
		Action:     m.Action,
		ExpiresAt:  millisPtr(m.ExpiresAt),
		Deleted:    boolInt(m.Deleted),
		DeletedAt:  millisPtr(m.DeletedAt),
	}
}

func millisPtr(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return millis(*t)
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (d dbMessage) model() *models.Message {
	m := &models.Message{
		ID:         d.ID,
		ReceivedAt: timeFromMillis(d.ReceivedAt),
		RemoteIP:   d.RemoteIP,
		LocalPort:  d.LocalPort,
		Domain:     d.Domain,
		Mailbox:    d.Mailbox,
		Recipient:  d.Recipient,
		Sender:     d.Sender,
		Subject:    d.Subject,
		RawHeaders: d.RawHeaders,
		Raw:        d.Raw,
		BodyText:   d.BodyText,
		BodyHTML:   d.BodyHTML,
		Size:       d.Size,
		PolicyID:   d.PolicyID,
		PolicyName: d.PolicyName,
		Action:     d.Action,
		ExpiresAt:  timePtrFromMillis(d.ExpiresAt),
		Deleted:    d.Deleted != 0,
		DeletedAt:  timePtrFromMillis(d.DeletedAt),
	}
	if d.Headers != "" {
		_ = json.Unmarshal([]byte(d.Headers), &m.Headers)
	}
	return m
}

// SaveMessage inserts or replaces a message.
func (s *Genji) SaveMessage(ctx context.Context, m *models.Message) error {
	if m.ID == "" {
		m.ID = models.NewID("msg")
	}
	normalizeMessage(m)
	dbm := messageToDB(m)

	s.mu.Lock()
	defer s.mu.Unlock()
	db := s.db.WithContext(ctx)

	err := db.Exec(`INSERT INTO messages (`+messageColumns+`, search_text)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		dbm.ID, dbm.ReceivedAt, dbm.RemoteIP, dbm.LocalPort, dbm.Domain, dbm.Mailbox,
		dbm.Recipient, dbm.Sender, dbm.Subject, dbm.Headers, dbm.RawHeaders, dbm.Raw,
		dbm.BodyText, dbm.BodyHTML, dbm.Size, dbm.PolicyID, dbm.PolicyName, dbm.Action,
		dbm.ExpiresAt, dbm.Deleted, dbm.DeletedAt, m.SearchText())
	if err == nil {
		return nil
	}
	if !genji.IsAlreadyExistsError(err) {
		return err
	}
	return db.Exec(`UPDATE messages SET
		received_at = ?, remote_ip = ?, local_port = ?, domain = ?, mailbox = ?,
		recipient = ?, sender = ?, subject = ?, headers = ?, raw_headers = ?, raw = ?,
		body_text = ?, body_html = ?, size_bytes = ?, policy_id = ?, policy_name = ?,
		policy_action = ?, expires_at = ?, deleted = ?, deleted_at = ?, search_text = ?
		WHERE id = ?`,
		dbm.ReceivedAt, dbm.RemoteIP, dbm.LocalPort, dbm.Domain, dbm.Mailbox,
		dbm.Recipient, dbm.Sender, dbm.Subject, dbm.Headers, dbm.RawHeaders, dbm.Raw,
		dbm.BodyText, dbm.BodyHTML, dbm.Size, dbm.PolicyID, dbm.PolicyName, dbm.Action,
		dbm.ExpiresAt, dbm.Deleted, dbm.DeletedAt, m.SearchText(), dbm.ID)
}

// GetMessage returns a message by id.
func (s *Genji) GetMessage(ctx context.Context, id string) (*models.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getMessageLocked(ctx, id)
}

func (s *Genji) getMessageLocked(ctx context.Context, id string) (*models.Message, error) {
	doc, err := s.db.WithContext(ctx).QueryDocument(
		`SELECT `+messageColumns+` FROM messages WHERE id = ?`, id)
	if err != nil {
		if genji.IsNotFoundError(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var dbm dbMessage
	if err := document.StructScan(doc, &dbm); err != nil {
		return nil, err
	}
	return dbm.model(), nil
}

func messageWhere(f models.MessageFilter) (string, []any) {
	var clauses []string
	var args []any

	if !f.IncludeDeleted {
		clauses = append(clauses, "deleted = 0")
	}
	if f.Query != "" {
		clauses = append(clauses, "search_text LIKE ?")
		args = append(args, "%"+strings.ToLower(f.Query)+"%")
	}
	if f.Domain != "" {
		clauses = append(clauses, "domain = ?")
		args = append(args, strings.ToLower(f.Domain))
	}
	if f.Mailbox != "" {
		clauses = append(clauses, "mailbox = ?")
		args = append(args, strings.ToLower(f.Mailbox))
	}
	if f.Action != "" {
		clauses = append(clauses, "policy_action = ?")
		args = append(args, f.Action)
	}
	if f.Port != 0 {
		clauses = append(clauses, "local_port = ?")
		args = append(args, f.Port)
	}
	if !f.Since.IsZero() {
		clauses = append(clauses, "received_at >= ?")
		args = append(args, millis(f.Since))
	}
	if !f.Until.IsZero() {
		clauses = append(clauses, "received_at <= ?")
		args = append(args, millis(f.Until))
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// ListMessages returns matching messages, newest first.
func (s *Genji) ListMessages(ctx context.Context, f models.MessageFilter) ([]*models.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	where, args := messageWhere(f)
	query := fmt.Sprintf(`SELECT %s FROM messages%s ORDER BY received_at DESC LIMIT %d OFFSET %d`,
		messageColumns, where, normalizeLimit(f.Limit), normalizeOffset(f.Offset))

	res, err := s.db.WithContext(ctx).Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	out := make([]*models.Message, 0)
	err = res.Iterate(func(d types.Document) error {
		var dbm dbMessage
		if err := document.StructScan(d, &dbm); err != nil {
			return err
		}
		out = append(out, dbm.model())
		return nil
	})
	return out, err
}

// CountMessages returns the number of messages matching f.
func (s *Genji) CountMessages(ctx context.Context, f models.MessageFilter) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	where, args := messageWhere(f)
	doc, err := s.db.WithContext(ctx).QueryDocument(
		`SELECT COUNT(*) AS n FROM messages`+where, args...)
	if err != nil {
		return 0, err
	}
	var n int
	if err := document.Scan(doc, &n); err != nil {
		return 0, err
	}
	return n, nil
}

// DeleteMessage removes a message by id.
func (s *Genji) DeleteMessage(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getMessageLocked(ctx, id); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Exec(`DELETE FROM messages WHERE id = ?`, id)
}

// DeleteExpired removes and returns messages whose expiry is at or before now.
func (s *Genji) DeleteExpired(ctx context.Context, now time.Time) ([]*models.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	db := s.db.WithContext(ctx)
	res, err := db.Query(`SELECT `+messageColumns+
		` FROM messages WHERE deleted = 0 AND expires_at > 0 AND expires_at <= ?`, millis(now))
	if err != nil {
		return nil, err
	}
	var expired []*models.Message
	err = res.Iterate(func(d types.Document) error {
		var dbm dbMessage
		if err := document.StructScan(d, &dbm); err != nil {
			return err
		}
		expired = append(expired, dbm.model())
		return nil
	})
	res.Close()
	if err != nil {
		return nil, err
	}
	if len(expired) == 0 {
		return expired, nil
	}
	if err := db.Exec(`DELETE FROM messages WHERE expires_at > 0 AND expires_at <= ?`, millis(now)); err != nil {
		return nil, err
	}
	return expired, nil
}

// SavePolicy inserts or updates a policy.
func (s *Genji) SavePolicy(ctx context.Context, p *models.Policy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	db := s.db.WithContext(ctx)
	now := time.Now().UTC()
	if p.ID == "" {
		p.ID = models.NewID("pol")
	}

	var existing *models.Policy
	if found, err := s.getPolicyLocked(ctx, p.ID); err == nil {
		existing = found
	}
	if existing != nil {
		p.CreatedAt = existing.CreatedAt
	} else {
		if p.CreatedAt.IsZero() {
			p.CreatedAt = now
		}
		if p.Priority == 0 {
			policies, err := s.listPoliciesLocked(ctx)
			if err != nil {
				return err
			}
			max := -1
			for _, existing := range policies {
				if existing.Priority > max {
					max = existing.Priority
				}
			}
			p.Priority = max + 1
		}
	}
	p.UpdatedAt = now

	dp := dbPolicy{
		ID:         p.ID,
		Name:       p.Name,
		Field:      p.Field,
		Pattern:    p.Pattern,
		Action:     p.Action,
		TTLSeconds: p.TTLSeconds,
		Priority:   p.Priority,
		Enabled:    boolInt(p.Enabled),
		CreatedAt:  millis(p.CreatedAt),
		UpdatedAt:  millis(p.UpdatedAt),
	}

	if existing == nil {
		err := db.Exec(`INSERT INTO policies
			(id, name, field_name, pattern, policy_action, ttl_seconds, priority, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			dp.ID, dp.Name, dp.Field, dp.Pattern, dp.Action, dp.TTLSeconds,
			dp.Priority, dp.Enabled, dp.CreatedAt, dp.UpdatedAt)
		if err == nil {
			return nil
		}
		if !genji.IsAlreadyExistsError(err) {
			return err
		}
	}
	return db.Exec(`UPDATE policies SET name = ?, field_name = ?, pattern = ?,
		policy_action = ?, ttl_seconds = ?, priority = ?, enabled = ?, created_at = ?, updated_at = ?
		WHERE id = ?`,
		dp.Name, dp.Field, dp.Pattern, dp.Action, dp.TTLSeconds, dp.Priority,
		dp.Enabled, dp.CreatedAt, dp.UpdatedAt, dp.ID)
}

func (s *Genji) getPolicyLocked(ctx context.Context, id string) (*models.Policy, error) {
	doc, err := s.db.WithContext(ctx).QueryDocument(
		`SELECT id, name, field_name, pattern, policy_action, ttl_seconds, priority, enabled,
		 created_at, updated_at FROM policies WHERE id = ?`, id)
	if err != nil {
		if genji.IsNotFoundError(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var dp dbPolicy
	if err := document.StructScan(doc, &dp); err != nil {
		return nil, err
	}
	return dp.model(), nil
}

func (d dbPolicy) model() *models.Policy {
	return &models.Policy{
		ID:         d.ID,
		Name:       d.Name,
		Field:      d.Field,
		Pattern:    d.Pattern,
		Action:     d.Action,
		TTLSeconds: d.TTLSeconds,
		Priority:   d.Priority,
		Enabled:    d.Enabled != 0,
		CreatedAt:  timeFromMillis(d.CreatedAt),
		UpdatedAt:  timeFromMillis(d.UpdatedAt),
	}
}

// GetPolicy returns a policy by id.
func (s *Genji) GetPolicy(ctx context.Context, id string) (*models.Policy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getPolicyLocked(ctx, id)
}

func (s *Genji) listPoliciesLocked(ctx context.Context) ([]*models.Policy, error) {
	res, err := s.db.WithContext(ctx).Query(
		`SELECT id, name, field_name, pattern, policy_action, ttl_seconds, priority, enabled,
		 created_at, updated_at FROM policies ORDER BY priority ASC`)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	out := make([]*models.Policy, 0)
	err = res.Iterate(func(d types.Document) error {
		var dp dbPolicy
		if err := document.StructScan(d, &dp); err != nil {
			return err
		}
		out = append(out, dp.model())
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority == out[j].Priority {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Priority < out[j].Priority
	})
	return out, nil
}

// ListPolicies returns policies ordered by ascending priority.
func (s *Genji) ListPolicies(ctx context.Context) ([]*models.Policy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listPoliciesLocked(ctx)
}

// DeletePolicy removes a policy by id.
func (s *Genji) DeletePolicy(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getPolicyLocked(ctx, id); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Exec(`DELETE FROM policies WHERE id = ?`, id)
}

// ReorderPolicies assigns priorities according to the provided id order.
func (s *Genji) ReorderPolicies(ctx context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	policies, err := s.listPoliciesLocked(ctx)
	if err != nil {
		return err
	}
	existing := make(map[string]bool, len(policies))
	for _, policy := range policies {
		existing[policy.ID] = true
	}
	for _, id := range ids {
		if !existing[id] {
			return ErrNotFound
		}
	}

	return s.db.WithContext(ctx).Update(func(tx *genji.Tx) error {
		now := millis(time.Now().UTC())
		for i, id := range ids {
			if err := tx.Exec(`UPDATE policies SET priority = ?, updated_at = ? WHERE id = ?`,
				i, now, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Genji) SaveEvent(ctx context.Context, ev events.Event) error {
	payload, err := json.Marshal(ev.Payload)
	if err != nil {
		return err
	}
	search := strings.ToLower(ev.Type + " " + ev.Severity + " " + string(payload))

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.WithContext(ctx).Exec(`INSERT INTO events
		(id, event_time, event_type, severity, payload, search_text)
		VALUES (?, ?, ?, ?, ?, ?)`,
		ev.ID, millis(ev.Timestamp), ev.Type, ev.Severity, string(payload), search)
}

// Write implements events.Sink.
func (s *Genji) Write(ev events.Event) error {
	return s.SaveEvent(context.Background(), ev)
}

func eventWhere(f models.EventFilter) (string, []any) {
	var clauses []string
	var args []any
	if f.Type != "" {
		clauses = append(clauses, "event_type = ?")
		args = append(args, f.Type)
	}
	if f.Severity != "" {
		clauses = append(clauses, "severity = ?")
		args = append(args, f.Severity)
	}
	if f.Query != "" {
		clauses = append(clauses, "search_text LIKE ?")
		args = append(args, "%"+strings.ToLower(f.Query)+"%")
	}
	if !f.Since.IsZero() {
		clauses = append(clauses, "event_time >= ?")
		args = append(args, millis(f.Since))
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// CountEvents returns the number of events matching f.
func (s *Genji) CountEvents(ctx context.Context, f models.EventFilter) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	where, args := eventWhere(f)
	doc, err := s.db.WithContext(ctx).QueryDocument(
		`SELECT COUNT(*) AS n FROM events`+where, args...)
	if err != nil {
		return 0, err
	}
	var n int
	if err := document.Scan(doc, &n); err != nil {
		return 0, err
	}
	return n, nil
}

// ListEvents returns matching events, newest first.
func (s *Genji) ListEvents(ctx context.Context, f models.EventFilter) ([]events.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	where, args := eventWhere(f)
	query := fmt.Sprintf(`SELECT %s FROM events%s ORDER BY event_time DESC LIMIT %d OFFSET %d`,
		eventColumns, where, normalizeLimit(f.Limit), normalizeOffset(f.Offset))

	res, err := s.db.WithContext(ctx).Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	out := make([]events.Event, 0)
	err = res.Iterate(func(d types.Document) error {
		var de dbEvent
		if err := document.StructScan(d, &de); err != nil {
			return err
		}
		ev := events.Event{
			ID:        de.ID,
			Timestamp: timeFromMillis(de.Timestamp),
			Type:      de.Type,
			Severity:  de.Severity,
		}
		if de.Payload != "" {
			_ = json.Unmarshal([]byte(de.Payload), &ev.Payload)
		}
		out = append(out, ev)
		return nil
	})
	return out, err
}

// GetUser returns a user by username.
func (s *Genji) GetUser(ctx context.Context, username string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, err := s.db.WithContext(ctx).QueryDocument(
		`SELECT username, password_hash, created_at FROM users WHERE username = ?`,
		strings.ToLower(username))
	if err != nil {
		if genji.IsNotFoundError(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var du dbUser
	if err := document.StructScan(doc, &du); err != nil {
		return nil, err
	}
	return &models.User{
		Username:     du.Username,
		PasswordHash: du.PasswordHash,
		CreatedAt:    timeFromMillis(du.CreatedAt),
	}, nil
}

// ListUsers returns administrator accounts ordered by username.
func (s *Genji) ListUsers(ctx context.Context) ([]*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res, err := s.db.WithContext(ctx).Query(
		`SELECT username, password_hash, created_at FROM users ORDER BY username ASC`)
	if err != nil {
		return nil, err
	}
	defer res.Close()
	out := make([]*models.User, 0)
	err = res.Iterate(func(d types.Document) error {
		var du dbUser
		if err := document.StructScan(d, &du); err != nil {
			return err
		}
		out = append(out, &models.User{
			Username:     du.Username,
			PasswordHash: du.PasswordHash,
			CreatedAt:    timeFromMillis(du.CreatedAt),
		})
		return nil
	})
	return out, err
}

// GetSetting returns a persisted key/value setting.
func (s *Genji) GetSetting(ctx context.Context, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, err := s.db.WithContext(ctx).QueryDocument(`SELECT setting_value FROM settings WHERE setting_key = ?`, key)
	if err != nil {
		if genji.IsNotFoundError(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	var row struct {
		Value string `genji:"setting_value"`
	}
	if err := document.StructScan(doc, &row); err != nil {
		return "", err
	}
	return row.Value, nil
}

// SetSetting inserts or replaces a key/value setting.
func (s *Genji) SetSetting(ctx context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	db := s.db.WithContext(ctx)
	err := db.Exec(`INSERT INTO settings (setting_key, setting_value) VALUES (?, ?)`, key, value)
	if err == nil {
		return nil
	}
	if !genji.IsAlreadyExistsError(err) {
		return err
	}
	return db.Exec(`UPDATE settings SET setting_value = ? WHERE setting_key = ?`, value, key)
}

// SaveUser inserts or replaces a user.
func (s *Genji) SaveUser(ctx context.Context, u *models.User) error {
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	du := dbUser{
		Username:     strings.ToLower(u.Username),
		PasswordHash: u.PasswordHash,
		CreatedAt:    millis(u.CreatedAt),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	db := s.db.WithContext(ctx)
	err := db.Exec(`INSERT INTO users (username, password_hash, created_at) VALUES (?, ?, ?)`,
		du.Username, du.PasswordHash, du.CreatedAt)
	if err == nil {
		return nil
	}
	if !genji.IsAlreadyExistsError(err) {
		return err
	}
	return db.Exec(`UPDATE users SET password_hash = ?, created_at = ? WHERE username = ?`,
		du.PasswordHash, du.CreatedAt, du.Username)
}

// SaveAPIKey inserts or replaces an API key.
func (s *Genji) SaveAPIKey(ctx context.Context, key *models.APIKey) error {
	if key.ID == "" {
		key.ID = models.NewID("key")
	}
	if key.CreatedAt.IsZero() {
		key.CreatedAt = time.Now().UTC()
	}
	dk := dbAPIKey{
		ID: key.ID, Name: key.Name, Prefix: key.Prefix, TokenHash: key.TokenHash,
		CreatedAt: millis(key.CreatedAt), ExpiresAt: millisPtr(key.ExpiresAt), CreatedBy: key.CreatedBy,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	db := s.db.WithContext(ctx)
	err := db.Exec(`INSERT INTO api_keys
		(id, name, prefix, token_hash, created_at, expires_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		dk.ID, dk.Name, dk.Prefix, dk.TokenHash, dk.CreatedAt, dk.ExpiresAt, dk.CreatedBy)
	if err == nil {
		return nil
	}
	if !genji.IsAlreadyExistsError(err) {
		return err
	}
	return db.Exec(`UPDATE api_keys SET name = ?, prefix = ?, token_hash = ?,
		created_at = ?, expires_at = ?, created_by = ? WHERE id = ?`,
		dk.Name, dk.Prefix, dk.TokenHash, dk.CreatedAt, dk.ExpiresAt, dk.CreatedBy, dk.ID)
}

// GetAPIKey returns an API key by id.
func (s *Genji) GetAPIKey(ctx context.Context, id string) (*models.APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getAPIKeyLocked(ctx, `id = ?`, id)
}

// GetAPIKeyByPrefix returns an API key by its public prefix.
func (s *Genji) GetAPIKeyByPrefix(ctx context.Context, prefix string) (*models.APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getAPIKeyLocked(ctx, `prefix = ?`, prefix)
}

func (s *Genji) getAPIKeyLocked(ctx context.Context, where string, value any) (*models.APIKey, error) {
	doc, err := s.db.WithContext(ctx).QueryDocument(
		`SELECT id, name, prefix, token_hash, created_at, expires_at, created_by
		 FROM api_keys WHERE `+where, value)
	if err != nil {
		if genji.IsNotFoundError(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var dk dbAPIKey
	if err := document.StructScan(doc, &dk); err != nil {
		return nil, err
	}
	return &models.APIKey{
		ID: dk.ID, Name: dk.Name, Prefix: dk.Prefix, TokenHash: dk.TokenHash,
		CreatedAt: timeFromMillis(dk.CreatedAt), ExpiresAt: timePtrFromMillis(dk.ExpiresAt),
		CreatedBy: dk.CreatedBy,
	}, nil
}

// ListAPIKeys returns API keys newest first.
func (s *Genji) ListAPIKeys(ctx context.Context) ([]*models.APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res, err := s.db.WithContext(ctx).Query(
		`SELECT id, name, prefix, token_hash, created_at, expires_at, created_by
		 FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer res.Close()
	out := make([]*models.APIKey, 0)
	err = res.Iterate(func(d types.Document) error {
		var dk dbAPIKey
		if err := document.StructScan(d, &dk); err != nil {
			return err
		}
		out = append(out, &models.APIKey{
			ID: dk.ID, Name: dk.Name, Prefix: dk.Prefix, TokenHash: dk.TokenHash,
			CreatedAt: timeFromMillis(dk.CreatedAt), ExpiresAt: timePtrFromMillis(dk.ExpiresAt),
			CreatedBy: dk.CreatedBy,
		})
		return nil
	})
	return out, err
}

// DeleteAPIKey removes an API key by id.
func (s *Genji) DeleteAPIKey(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getAPIKeyLocked(ctx, `id = ?`, id); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Exec(`DELETE FROM api_keys WHERE id = ?`, id)
}

// SaveWebDAVUser inserts or replaces a WebDAV credential.
func (s *Genji) SaveWebDAVUser(ctx context.Context, user *models.WebDAVUser) error {
	user.Username = strings.ToLower(strings.TrimSpace(user.Username))
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now().UTC()
	}
	du := dbWebDAVUser{Username: user.Username, PasswordHash: user.PasswordHash, CreatedAt: millis(user.CreatedAt)}
	s.mu.Lock()
	defer s.mu.Unlock()
	db := s.db.WithContext(ctx)
	err := db.Exec(`INSERT INTO webdav_users (username, password_hash, created_at) VALUES (?, ?, ?)`,
		du.Username, du.PasswordHash, du.CreatedAt)
	if err == nil {
		return nil
	}
	if !genji.IsAlreadyExistsError(err) {
		return err
	}
	return db.Exec(`UPDATE webdav_users SET password_hash = ?, created_at = ? WHERE username = ?`,
		du.PasswordHash, du.CreatedAt, du.Username)
}

// GetWebDAVUser returns a WebDAV credential by username.
func (s *Genji) GetWebDAVUser(ctx context.Context, username string) (*models.WebDAVUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getWebDAVUserLocked(ctx, username)
}

func (s *Genji) getWebDAVUserLocked(ctx context.Context, username string) (*models.WebDAVUser, error) {
	doc, err := s.db.WithContext(ctx).QueryDocument(
		`SELECT username, password_hash, created_at FROM webdav_users WHERE username = ?`,
		strings.ToLower(strings.TrimSpace(username)))
	if err != nil {
		if genji.IsNotFoundError(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var du dbWebDAVUser
	if err := document.StructScan(doc, &du); err != nil {
		return nil, err
	}
	return &models.WebDAVUser{Username: du.Username, PasswordHash: du.PasswordHash, CreatedAt: timeFromMillis(du.CreatedAt)}, nil
}

// ListWebDAVUsers returns WebDAV credentials ordered by username.
func (s *Genji) ListWebDAVUsers(ctx context.Context) ([]*models.WebDAVUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res, err := s.db.WithContext(ctx).Query(
		`SELECT username, password_hash, created_at FROM webdav_users ORDER BY username ASC`)
	if err != nil {
		return nil, err
	}
	defer res.Close()
	out := make([]*models.WebDAVUser, 0)
	err = res.Iterate(func(d types.Document) error {
		var du dbWebDAVUser
		if err := document.StructScan(d, &du); err != nil {
			return err
		}
		out = append(out, &models.WebDAVUser{Username: du.Username, PasswordHash: du.PasswordHash, CreatedAt: timeFromMillis(du.CreatedAt)})
		return nil
	})
	return out, err
}

// DeleteWebDAVUser removes a WebDAV credential.
func (s *Genji) DeleteWebDAVUser(ctx context.Context, username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.ToLower(strings.TrimSpace(username))
	if _, err := s.getWebDAVUserLocked(ctx, key); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Exec(`DELETE FROM webdav_users WHERE username = ?`, key)
}

// Close closes the database.
func (s *Genji) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}
