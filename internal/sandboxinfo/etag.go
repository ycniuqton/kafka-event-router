package sandboxinfo

import (
	"strings"

	"github.com/eventrouter/router/internal/platform/api"
)

// HeaderIfNoneMatch is the conditional GET request header.
const HeaderIfNoneMatch api.Header = "If-None-Match"

// ETagToken names If-None-Match syntax: the wildcard and the list separator.
type ETagToken string

const (
	ETagAny       ETagToken = "*"
	ETagSeparator ETagToken = ","
)

// ETagMatch compares an If-None-Match list with the current tag (weak tags compare by value).
type ETagMatch struct{}

// Matches reports whether header names etag or is the wildcard.
func (ETagMatch) Matches(header, etag string) bool {
	for _, raw := range strings.Split(header, string(ETagSeparator)) {
		tag := strings.TrimPrefix(strings.TrimSpace(raw), string(api.WeakETagPrefix))
		if tag == string(ETagAny) || tag == etag {
			return true
		}
	}
	return false
}
