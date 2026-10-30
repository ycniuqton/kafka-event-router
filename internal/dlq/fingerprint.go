package dlq

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/eventrouter/router/internal/dlq/record"
)

// Placeholder replaces a volatile part of an error message (design 00001 §3.1).
type Placeholder string

const (
	PlaceholderTimestamp Placeholder = "<ts>"
	PlaceholderUUID      Placeholder = "<uuid>"
	PlaceholderID        Placeholder = "<id>"
	PlaceholderHex       Placeholder = "<hex>"
	PlaceholderNumber    Placeholder = "<n>"
)

// FingerprintLimit bounds patterns and ids.
type FingerprintLimit int

const (
	MaxPatternRunes FingerprintLimit = 240
	FingerprintHex  FingerprintLimit = 16
)

// FingerprintSeparator joins the hashed parts so "a"+"bc" never equals "ab"+"c".
type FingerprintSeparator string

const FingerprintJoin FingerprintSeparator = "\x1f"

// TokenFilter decides whether one matched token is volatile (nil accepts every match).
type TokenFilter interface {
	Volatile(token string) bool
}

// NormalizationRule replaces the matches of Pattern that Filter accepts with Placeholder.
type NormalizationRule struct {
	Pattern     *regexp.Regexp
	Placeholder Placeholder
	Filter      TokenFilter
}

// Apply rewrites s.
func (r NormalizationRule) Apply(s string) string {
	return r.Pattern.ReplaceAllStringFunc(s, func(token string) string {
		if r.Filter != nil && !r.Filter.Volatile(token) {
			return token
		}
		return string(r.Placeholder)
	})
}

// HexRun accepts hex tokens of at least HexRunMin characters mixing digits and letters (short words like
// "deadbeef" without digits and plain numbers are left to other rules).
type HexRun struct{}

// Volatile implements TokenFilter.
func (HexRun) Volatile(token string) bool {
	t := strings.TrimPrefix(strings.ToLower(token), "0x")
	return len(t) >= int(HexRunMin) && strings.ContainsAny(t, "0123456789") && strings.ContainsAny(t, "abcdef")
}

// HexRunMin is the shortest hex run treated as an id.
const HexRunMin FingerprintLimit = 8

// Whitespace is the collapsed form of any whitespace run.
const Whitespace Placeholder = " "

// DefaultNormalizationRules are applied in order: the most specific shapes first, plain numbers last.
var DefaultNormalizationRules = []NormalizationRule{
	{Pattern: regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?`), Placeholder: PlaceholderTimestamp},
	{Pattern: regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`), Placeholder: PlaceholderUUID},
	{Pattern: regexp.MustCompile(`(?i)\b(0x)?[0-9a-f]{8,}\b`), Placeholder: PlaceholderHex, Filter: HexRun{}},
	{Pattern: regexp.MustCompile(`\b[A-Za-z]{1,12}[_-][A-Za-z0-9]*\d[A-Za-z0-9_-]*\b`), Placeholder: PlaceholderID},
	{Pattern: regexp.MustCompile(`\d+(\.\d+)?`), Placeholder: PlaceholderNumber},
	{Pattern: regexp.MustCompile(`\s+`), Placeholder: Whitespace},
}

// MessageNormalizer turns an error message into its cause pattern: volatile ids and numbers become placeholders.
type MessageNormalizer struct {
	Rules []NormalizationRule
}

// NewMessageNormalizer returns the default normalizer.
func NewMessageNormalizer() MessageNormalizer {
	return MessageNormalizer{Rules: DefaultNormalizationRules}
}

// Pattern returns the normalized message, trimmed and cut to MaxPatternRunes.
func (n MessageNormalizer) Pattern(message string) string {
	out := message
	for _, r := range n.Rules {
		out = r.Apply(out)
	}
	return RuneCutter{}.Cut(strings.TrimSpace(out), int(MaxPatternRunes))
}

// RuneCutter cuts strings by runes.
type RuneCutter struct{}

// Cut returns s limited to max runes.
func (RuneCutter) Cut(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

// Fingerprint identifies one cause of dead letters: route + step + error type + message pattern.
type Fingerprint struct {
	ID      string
	Pattern string
}

// Fingerprinter computes fingerprints (stable across versions, partitions and record payloads).
type Fingerprinter struct {
	Normalizer MessageNormalizer
}

// NewFingerprinter returns a fingerprinter with the default rules.
func NewFingerprinter() Fingerprinter {
	return Fingerprinter{Normalizer: NewMessageNormalizer()}
}

// Of returns the fingerprint of a failure of routeID.
func (f Fingerprinter) Of(routeID string, failure record.Failure) Fingerprint {
	pattern := f.Normalizer.Pattern(failure.Message)
	parts := strings.Join([]string{routeID, failure.Step, string(failure.ErrorType), pattern}, string(FingerprintJoin))
	sum := sha256.Sum256([]byte(parts))
	return Fingerprint{ID: hex.EncodeToString(sum[:])[:FingerprintHex], Pattern: pattern}
}
