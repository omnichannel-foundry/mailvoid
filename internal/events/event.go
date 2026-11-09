// Package events implements the event-first core used by every subsystem.
package events

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Event types emitted by MailVoid.
const (
	TypeSystemStartup  = "system.startup"
	TypeSystemShutdown = "system.shutdown"
	TypeSystemCapture  = "system.capture"

	TypeSMTPConnected = "smtp.connected"
	TypeSMTPClosed    = "smtp.closed"
	TypeSMTPMailFrom  = "smtp.mail_from"
	TypeSMTPRcptTo    = "smtp.rcpt_to"
	TypeSMTPData      = "smtp.data"
	TypeSMTPError     = "smtp.error"

	TypeMessageReceived = "message.received"
	TypeMessageDropped  = "message.dropped"
	TypeMessageDeleted  = "message.deleted"
	TypeMessageError    = "message.error"
	TypeSearchError     = "search.error"

	TypePolicyMatched   = "policy.matched"
	TypePolicyCreated   = "policy.created"
	TypePolicyUpdated   = "policy.updated"
	TypePolicyDeleted   = "policy.deleted"
	TypePolicyReordered = "policy.reordered"

	TypeAuthLoginSuccess    = "auth.login.success"
	TypeAuthLoginFailed     = "auth.login.failed"
	TypeAuthLogout          = "auth.logout"
	TypeAuthPasswordChanged = "auth.password.changed"

	TypeAPIKeyCreated     = "api_key.created"
	TypeAPIKeyDeleted     = "api_key.deleted"
	TypeWebDAVUserCreated = "webdav_user.created"
	TypeWebDAVUserDeleted = "webdav_user.deleted"
)

// Severity levels.
const (
	SeverityDebug = "debug"
	SeverityInfo  = "info"
	SeverityWarn  = "warn"
	SeverityError = "error"
)

// Event is the structured record persisted and broadcast for every lifecycle
// action.
type Event struct {
	ID        string         `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Type      string         `json:"type"`
	Severity  string         `json:"severity"`
	Payload   map[string]any `json:"payload,omitempty"`
}

// New builds an event with a generated id and current UTC timestamp.
func New(typ, severity string, payload map[string]any) Event {
	return Event{
		ID:        newID(),
		Timestamp: time.Now().UTC(),
		Type:      typ,
		Severity:  severity,
		Payload:   payload,
	}
}

// MarshalJSON keeps payloads deterministic for storage and transport.
func (e Event) MarshalJSON() ([]byte, error) {
	type alias Event
	return json.Marshal(alias(e))
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "ev_" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return "ev_" + hex.EncodeToString(b)
}
