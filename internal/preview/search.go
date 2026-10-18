package preview

import (
	"bytes"
	"strings"
)

// SampleSearcher matches samples by case-insensitive text over key, value and headers.
type SampleSearcher struct{}

// Filter returns the records matching q; an empty query matches everything.
func (s SampleSearcher) Filter(recs []SampleRecord, q string) []SampleRecord {
	needle := []byte(strings.ToLower(strings.TrimSpace(q)))
	if len(needle) == 0 {
		return recs
	}
	out := make([]SampleRecord, 0, len(recs))
	for _, r := range recs {
		if s.Match(r, needle) {
			out = append(out, r)
		}
	}
	return out
}

// Match reports whether the lower-cased needle occurs in the record.
func (SampleSearcher) Match(r SampleRecord, needle []byte) bool {
	if bytes.Contains(bytes.ToLower(r.Key), needle) || bytes.Contains(bytes.ToLower(r.Value), needle) {
		return true
	}
	for _, h := range r.Headers {
		if bytes.Contains(bytes.ToLower([]byte(h.Key)), needle) || bytes.Contains(bytes.ToLower(h.Value), needle) {
			return true
		}
	}
	return false
}
