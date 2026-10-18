// Package sandboxtest provides deterministic host services, salts and a manager factory for
// sandbox tests and tools; production binds the engine's ctxhelpers instead.
package sandboxtest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/eventrouter/router/pkg/sandbox/host"
)

// MaskSymbol is a marker used by the reference services.
type MaskSymbol string

const (
	maskSymbol MaskSymbol = "*"
	emailAt    MaskSymbol = "@"
	saltPrefix MaskSymbol = "salt-"
)

// visibleTail is how many characters last4 keeps.
type visibleTail int

const last4Visible visibleTail = 4

// Services is a minimal, deterministic HostServices for tests.
type Services struct {
	Salt   []byte
	Tables map[string]map[string]any
}

func (s Services) Masker() host.Masker               { return s }
func (s Services) Hasher() host.Hasher               { return s }
func (s Services) DateFormatter() host.DateFormatter { return s }
func (s Services) Lookups() host.LookupResolver      { return s }

// Mask applies the reference mask styles.
func (s Services) Mask(v any, style host.MaskStyle) (any, error) {
	if v == nil {
		return nil, nil
	}
	text := fmt.Sprint(v)
	switch style {
	case host.MaskFull:
		return strings.Repeat(string(maskSymbol), len([]rune(text))), nil
	case host.MaskLast4:
		r := []rune(text)
		cut := max(len(r)-int(last4Visible), 0)
		return strings.Repeat(string(maskSymbol), cut) + string(r[cut:]), nil
	}
	at := strings.LastIndex(text, string(emailAt))
	return strings.Repeat(string(maskSymbol), max(at, 0)) + text[max(at, 0):], nil
}

// Hash is hex SHA-256(salt || text).
func (s Services) Hash(_ context.Context, v any) (string, error) {
	sum := sha256.Sum256(append(append([]byte(nil), s.Salt...), fmt.Sprint(v)...))
	return hex.EncodeToString(sum[:]), nil
}

// Format renders RFC 3339 in the zone (layout ignored).
func (s Services) Format(t time.Time, _ string, tz string) (string, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return "", err
	}
	return t.In(loc).Format(time.RFC3339), nil
}

// Lookup reads Tables.
func (s Services) Lookup(table, key string) (any, bool) {
	v, ok := s.Tables[table][key]
	return v, ok
}

// Binder binds Services with the team salt.
type Binder struct {
	Tables map[string]map[string]any
}

func (b Binder) Bind(_ context.Context, _ string, salt []byte) (host.HostServices, error) {
	return Services{Salt: salt, Tables: b.Tables}, nil
}

// Salts returns "salt-<team>" for every team.
type Salts struct{}

func (Salts) Salt(_ context.Context, teamID string) ([]byte, error) {
	return []byte(string(saltPrefix) + teamID), nil
}
