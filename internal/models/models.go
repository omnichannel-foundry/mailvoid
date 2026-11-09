// Package models contains the domain types shared by storage, policy, SMTP,
// events, and the web layer.
package models

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
)

// Policy actions.
const (
	ActionKeep = "keep"
	ActionTTL  = "ttl"
	ActionDrop = "drop"
)

// Policy target fields.
const (
	FieldSender    = "sender"
	FieldRecipient = "recipient"
	FieldMailbox   = "mailbox"
	FieldDomain    = "domain"
	FieldSubject   = "subject"
	FieldRemoteIP  = "remote_ip"
	FieldLocalPort = "local_port"
	FieldAny       = "any"
)

// PolicyFields lists the fields accepted by the policy engine.
var PolicyFields = []string{
	FieldSender, FieldRecipient, FieldMailbox, FieldDomain,
	FieldSubject, FieldRemoteIP, FieldLocalPort, FieldAny,
}

// PolicyActions lists the actions accepted by the policy engine.
var PolicyActions = []string{ActionKeep, ActionTTL, ActionDrop}

// NewID returns a short, random, prefixed identifier.
func NewID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return prefix + "_" + hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return prefix + "_" + hex.EncodeToString(b)
}

// Message is a captured email together with the SMTP envelope metadata needed
// to inspect it later.
type Message struct {
	ID         string              `json:"id"`
	ReceivedAt time.Time           `json:"received_at"`
	RemoteIP   string              `json:"remote_ip"`
	LocalPort  int                 `json:"local_port"`
	Domain     string              `json:"domain"`
	Mailbox    string              `json:"mailbox"`
	Recipient  string              `json:"recipient"`
	Sender     string              `json:"sender"`
	Subject    string              `json:"subject"`
	Headers    map[string][]string `json:"headers,omitempty"`
	RawHeaders string              `json:"raw_headers,omitempty"`
	Raw        string              `json:"raw,omitempty"`
	BodyText   string              `json:"body_text,omitempty"`
	BodyHTML   string              `json:"body_html,omitempty"`
	Size       int64               `json:"size"`
	PolicyID   string              `json:"policy_id,omitempty"`
	PolicyName string              `json:"policy_name,omitempty"`
	Action     string              `json:"action"`
	ExpiresAt  *time.Time          `json:"expires_at,omitempty"`
	Deleted    bool                `json:"deleted"`
	DeletedAt  *time.Time          `json:"deleted_at,omitempty"`
}

// Address returns the recipient in mailbox@domain form.
func (m *Message) Address() string {
	if m.Mailbox == "" && m.Domain == "" {
		return m.Recipient
	}
	return m.Mailbox + "@" + m.Domain
}

// Policy is an ordered regular-expression retention rule.
type Policy struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Field      string    `json:"field"`
	Pattern    string    `json:"pattern"`
	Action     string    `json:"action"`
	TTLSeconds int64     `json:"ttl_seconds"`
	Priority   int       `json:"priority"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TTL returns the policy lifetime as a duration.
func (p Policy) TTL() time.Duration {
	return time.Duration(p.TTLSeconds) * time.Second
}

// User is an administrator account.
type User struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// APIKey is a managed bearer credential. The plaintext token is returned only
// once at creation; only its hash is stored.
type APIKey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	TokenHash string     `json:"-"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedBy string     `json:"created_by,omitempty"`
}

// WebDAVUser is a managed Basic-auth credential for the read-only WebDAV tree.
type WebDAVUser struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// MessageFilter narrows message queries.
type MessageFilter struct {
	Query          string
	Domain         string
	Mailbox        string
	Action         string
	Port           int
	Since          time.Time
	Until          time.Time
	Limit          int
	Offset         int
	IncludeDeleted bool
}

// EventFilter narrows event queries.
type EventFilter struct {
	Type     string
	Severity string
	Query    string
	Since    time.Time
	Limit    int
	Offset   int
}

// SearchText returns the lower-cased text used for substring queries.
func (m *Message) SearchText() string {
	parts := []string{
		m.Sender, m.Recipient, m.Mailbox, m.Domain, m.Subject, m.RemoteIP,
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// NormalizeMailbox lower-cases a domain and trims whitespace from a mailbox.
func NormalizeMailbox(mailbox, domain string) (string, string) {
	return strings.TrimSpace(mailbox), strings.ToLower(strings.TrimSpace(domain))
}

// SplitAddress splits an SMTP address into mailbox and domain.
func SplitAddress(addr string) (mailbox, domain string) {
	addr = strings.TrimSpace(addr)
	addr = strings.TrimPrefix(addr, "<")
	addr = strings.TrimSuffix(addr, ">")
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return strings.ToLower(addr), ""
	}
	return strings.ToLower(addr[:at]), strings.ToLower(addr[at+1:])
}
