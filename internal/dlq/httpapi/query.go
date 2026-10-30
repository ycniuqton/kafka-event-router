package httpapi

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/dlq/record"
)

// QueryParam names query parameters.
type QueryParam string

const (
	QueryLimit           QueryParam = "limit"
	QueryCursor          QueryParam = "cursor"
	QueryErrorType       QueryParam = "errorType"
	QuerySince           QueryParam = "since"
	QueryUntil           QueryParam = "until"
	QueryIncludeResolved QueryParam = "includeResolved"
	QueryRouteID         QueryParam = "routeId"
	QueryIDs             QueryParam = "ids"
	QueryFormat          QueryParam = "format"
	QueryStep            QueryParam = "step"
	QueryVersion         QueryParam = "version"
	QueryText            QueryParam = "q"
	QueryClass           QueryParam = "class"
	QueryFingerprint     QueryParam = "fingerprint"
	QueryStatus          QueryParam = "status"
)

// ListSeparator separates multi-value query parameters.
type ListSeparator string

const listSep ListSeparator = ","

// QueryParser parses list/export filters (design §4.1).
type QueryParser struct{}

// Parse returns the query; bad values are 400 problems pointing at /query/<name>.
func (p QueryParser) Parse(r *http.Request) (dlq.Query, error) {
	v := r.URL.Query()
	q := dlq.Query{Cursor: v.Get(string(QueryCursor)), IncludeResolved: v.Get(string(QueryIncludeResolved)) == strconv.FormatBool(true),
		Step: strings.TrimSpace(v.Get(string(QueryStep))), Text: strings.TrimSpace(v.Get(string(QueryText)))}
	var err error
	if q.Limit, err = p.limit(v.Get(string(QueryLimit))); err != nil {
		return dlq.Query{}, err
	}
	if q.VersionNo, err = p.version(v.Get(string(QueryVersion)), dlq.PathVersion); err != nil {
		return dlq.Query{}, err
	}
	if q.ErrorTypes, err = p.errorTypes(v.Get(string(QueryErrorType))); err != nil {
		return dlq.Query{}, err
	}
	if q.Since, err = p.time(v.Get(string(QuerySince)), dlq.PathSince); err != nil {
		return dlq.Query{}, err
	}
	if q.Until, err = p.time(v.Get(string(QueryUntil)), dlq.PathUntil); err != nil {
		return dlq.Query{}, err
	}
	return CauseFilters{}.Apply(q, v.Get(string(QueryClass)), v.Get(string(QueryFingerprint)))
}

// CauseFilters parses the workflow filters: error classes and the cause fingerprint (design 00001 §3.3).
type CauseFilters struct{}

// FingerprintPattern is the shape of a fingerprint id.
var FingerprintPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// Apply validates and sets classes and fingerprint on q.
func (CauseFilters) Apply(q dlq.Query, classes, fingerprint string) (dlq.Query, error) {
	for i, part := range (Values{}).Split(classes) {
		c := record.ErrorClass(part)
		if !c.Valid() {
			return dlq.Query{}, dlq.ErrInvalidFilter.At(dlq.PathClass.Indexed(i))
		}
		q.Classes = append(q.Classes, c)
	}
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint != "" && !FingerprintPattern.MatchString(fingerprint) {
		return dlq.Query{}, dlq.ErrInvalidFilter.At(string(dlq.PathFingerprint))
	}
	q.Fingerprint = fingerprint
	return q, nil
}

func (QueryParser) limit(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > int(dlq.MaxPage) {
		return 0, dlq.ErrInvalidFilter.At(string(dlq.PathLimit))
	}
	return n, nil
}

// version parses a positive route version number (empty = any version).
func (QueryParser) version(raw string, path dlq.RequestPath) (int, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, dlq.ErrInvalidFilter.At(string(path))
	}
	return n, nil
}

func (QueryParser) errorTypes(raw string) ([]record.ErrorType, error) {
	out := []record.ErrorType{}
	for i, part := range (Values{}).Split(raw) {
		t := record.ErrorType(part)
		if !t.Valid() {
			return nil, dlq.ErrInvalidFilter.At(dlq.PathErrType.Indexed(i))
		}
		out = append(out, t)
	}
	return out, nil
}

func (QueryParser) time(raw string, path dlq.RequestPath) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, dlq.ErrInvalidFilter.At(string(path))
	}
	return t, nil
}

// Values splits comma-separated query values.
type Values struct{}

// Split returns the non-empty trimmed parts of raw.
func (Values) Split(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, string(listSep)) {
		if s := strings.TrimSpace(part); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Set returns the parts of raw as a set.
func (v Values) Set(raw string) map[string]bool {
	out := map[string]bool{}
	for _, s := range v.Split(raw) {
		out[s] = true
	}
	return out
}
