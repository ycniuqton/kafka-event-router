package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
)

// ETagMarker names entity-tag syntax markers; weak tags are accepted and compared by value.
type ETagMarker string

const WeakETagPrefix ETagMarker = "W/"

var (
	// ErrPreconditionRequired is returned when a versioned mutation lacks If-Match (428).
	ErrPreconditionRequired = problem.NewError(problem.KindPreconditionRequired,
		"This change needs the version you edited. Reload the resource and retry.").At("/If-Match")
	// ErrBadIfMatch is returned for an unparsable If-Match header.
	ErrBadIfMatch = problem.NewError(problem.KindMalformedBody, "If-Match must be a quoted version such as \"7\".").At("/If-Match")
)

// Precondition reads the If-Match header of versioned mutations.
type Precondition struct{}

// Require returns the expected row version or 428/400.
func (Precondition) Require(r *http.Request) (store.RowVersion, error) {
	raw := strings.TrimSpace(r.Header.Get(string(HeaderIfMatch)))
	if raw == "" {
		return 0, ErrPreconditionRequired
	}
	return ETag(raw).Version()
}

// Compare fails with a 412 when the expected version differs from the current one.
func (Precondition) Compare(expected, current store.RowVersion) error {
	if expected != current {
		return &store.PreconditionFailedError{Current: current}
	}
	return nil
}

// ETag is a raw entity-tag header value.
type ETag string

// Version parses `"7"` or `W/"7"` into a row version.
func (e ETag) Version() (store.RowVersion, error) {
	s := strings.TrimPrefix(string(e), string(WeakETagPrefix))
	unquoted, err := strconv.Unquote(s)
	if err != nil {
		return 0, ErrBadIfMatch
	}
	v, err := strconv.ParseInt(unquoted, 10, 64)
	if err != nil {
		return 0, ErrBadIfMatch
	}
	return store.RowVersion(v), nil
}
