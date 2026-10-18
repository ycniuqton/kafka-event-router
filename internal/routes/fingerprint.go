package routes

import (
	"encoding/json"

	"github.com/eventrouter/router/pkg/config"
)

// ConfigFingerprint hashes route documents canonically so equal configs compare equal.
type ConfigFingerprint struct{}

// Of returns the sha256 hex of the RFC 8785 canonical JSON of raw.
func (ConfigFingerprint) Of(raw json.RawMessage) (string, error) {
	h, err := config.Canonicalizer{}.Hash(raw)
	return string(h), err
}

// Same reports whether two documents are canonically identical (unparseable → false).
func (f ConfigFingerprint) Same(a, b json.RawMessage) bool {
	if a == nil || b == nil {
		return false
	}
	ha, errA := f.Of(a)
	hb, errB := f.Of(b)
	return errA == nil && errB == nil && ha == hb
}
