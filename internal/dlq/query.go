package dlq

import (
	"strings"
	"time"

	"github.com/eventrouter/router/internal/dlq/record"
)

// Query filters and pages dead letters; zero Since/Until mean unbounded.
type Query struct {
	ErrorTypes      []record.ErrorType
	Step            string
	VersionNo       int
	Text            string
	Since           time.Time
	Until           time.Time
	IncludeResolved bool
	Limit           int
	Cursor          string
	Classes         []record.ErrorClass
	Fingerprint     string
}

// PageSize returns Limit clamped to [1, MaxPage], DefaultPage when unset.
func (q Query) PageSize() int {
	switch {
	case q.Limit <= 0:
		return int(DefaultPage)
	case q.Limit > int(MaxPage):
		return int(MaxPage)
	}
	return q.Limit
}

// Matches reports whether r passes the error-type, step, version, text and time filters.
func (q Query) Matches(r record.Record) bool {
	if !q.matchesType(r.Failure.ErrorType) || !q.matchesFailure(r.Failure) || !q.matchesCause(r.Failure) {
		return false
	}
	at := r.Failure.FailedAt
	if !q.Since.IsZero() && at.Before(q.Since) {
		return false
	}
	return q.Until.IsZero() || !at.After(q.Until)
}

// matchesFailure applies the P2 filters: exact step id, exact version, case-insensitive message text.
func (q Query) matchesFailure(f record.Failure) bool {
	if q.Step != "" && f.Step != q.Step {
		return false
	}
	if q.VersionNo > 0 && f.VersionNo != q.VersionNo {
		return false
	}
	return q.Text == "" || strings.Contains(strings.ToLower(f.Message), strings.ToLower(q.Text))
}

// matchesCause applies the workflow filters: error class and cause fingerprint (design 00001 §3.3).
func (q Query) matchesCause(f record.Failure) bool {
	if len(q.Classes) > 0 && !(ClassSet{}).Has(q.Classes, f.ErrorType.Class()) {
		return false
	}
	return q.Fingerprint == "" || NewFingerprinter().Of(f.RouteID, f).ID == q.Fingerprint
}

// ClassSet tests class lists.
type ClassSet struct{}

// Has reports whether c is in list.
func (ClassSet) Has(list []record.ErrorClass, c record.ErrorClass) bool {
	for _, x := range list {
		if x == c {
			return true
		}
	}
	return false
}

func (q Query) matchesType(t record.ErrorType) bool {
	if len(q.ErrorTypes) == 0 {
		return true
	}
	for _, k := range q.ErrorTypes {
		if k == t {
			return true
		}
	}
	return false
}

// WithoutPaging returns the filters only (selection by match ignores cursor and limit).
func (q Query) WithoutPaging() Query {
	q.Cursor, q.Limit = "", 0
	return q
}

// Selection picks the dead letters an action applies to: explicit ids or every match of a query.
type Selection struct {
	IDs           []string
	Match         *Query
	ExpectedCount *int
}

// Validate requires exactly one of IDs and Match.
func (s Selection) Validate() error {
	hasIDs, hasMatch := len(s.IDs) > 0, s.Match != nil
	if hasIDs == hasMatch {
		return ErrSelectionShape.At(string(PathRoot))
	}
	return nil
}
