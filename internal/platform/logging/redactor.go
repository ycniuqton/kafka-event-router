package logging

import (
	"log/slog"
	"strings"
)

// SensitiveKey names attribute or field keys whose values must never reach logs or audit diffs.
type SensitiveKey string

const (
	SensitiveSecret       SensitiveKey = "secret"
	SensitivePayload      SensitiveKey = "payload"
	SensitivePassword     SensitiveKey = "password"
	SensitiveToken        SensitiveKey = "token"
	SensitivePrivateKey   SensitiveKey = "private_key"
	SensitiveCredentials  SensitiveKey = "credentials"
	SensitiveClientSecret SensitiveKey = "client_secret"
	SensitiveValue        SensitiveKey = "value_b64"
)

// RedactedMarker replaces values that are kept structurally but must not be shown.
type RedactedMarker string

const Redacted RedactedMarker = "[REDACTED]"

// Secret wraps a value so it renders as a redaction marker in any slog output.
type Secret string

// LogValue implements slog.LogValuer.
func (Secret) LogValue() slog.Value {
	return slog.StringValue(string(Redacted))
}

// Redactor decides which keys are sensitive and drops them from log records and diffs.
type Redactor struct {
	keys []SensitiveKey
}

// NewRedactor returns a Redactor with the platform's standard sensitive keys.
func NewRedactor(extra ...SensitiveKey) *Redactor {
	keys := []SensitiveKey{SensitiveSecret, SensitivePayload, SensitivePassword, SensitiveToken,
		SensitivePrivateKey, SensitiveCredentials, SensitiveClientSecret, SensitiveValue}
	return &Redactor{keys: append(keys, extra...)}
}

// IsSensitive reports whether a key (case-insensitive, substring match) carries secret data.
func (r *Redactor) IsSensitive(key string) bool {
	lower := strings.ToLower(key)
	for _, k := range r.keys {
		if strings.Contains(lower, string(k)) {
			return true
		}
	}
	return false
}

// ReplaceAttr is a slog.HandlerOptions hook that drops sensitive attributes.
func (r *Redactor) ReplaceAttr(_ []string, a slog.Attr) slog.Attr {
	if r.IsSensitive(a.Key) {
		return slog.Attr{}
	}
	return a
}
