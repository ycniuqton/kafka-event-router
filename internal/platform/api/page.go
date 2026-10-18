package api

import (
	"net/http"
	"strconv"

	"github.com/eventrouter/router/internal/store"
)

// PageParam names the pagination query parameters.
type PageParam string

const (
	ParamLimit  PageParam = "limit"
	ParamCursor PageParam = "cursor"
)

// ErrBadLimit is returned for non-numeric or non-positive limits (422).
var ErrBadLimit = NewValidationError().Add("/limit", "range", "limit must be a whole number between 1 and 200.")

// PageRequest is a parsed `?limit=&cursor=` pair.
type PageRequest struct {
	Limit int
	After *store.Cursor
}

// PageParser parses and verifies page requests and encodes next cursors.
type PageParser struct {
	codec *store.CursorCodec
}

// NewPageParser builds a parser over the HMAC cursor codec.
func NewPageParser(codec *store.CursorCodec) *PageParser {
	return &PageParser{codec: codec}
}

// Parse reads limit (default 50, clamped to 200, ≤0 → 422) and a tamper-checked cursor (→ 400 /cursor).
func (p *PageParser) Parse(r *http.Request) (PageRequest, error) {
	q := r.URL.Query()
	req := PageRequest{Limit: int(store.DefaultPageLimit)}
	if raw := q.Get(string(ParamLimit)); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return PageRequest{}, ErrBadLimit
		}
		req.Limit = min(n, int(store.MaxPageLimit))
	}
	if raw := q.Get(string(ParamCursor)); raw != "" {
		cur, err := p.codec.Decode(raw)
		if err != nil {
			return PageRequest{}, err
		}
		req.After = &cur
	}
	return req, nil
}

// Next encodes the cursor of the following page, or nil on the last page.
func (p *PageParser) Next(next *store.Cursor) *string {
	if next == nil {
		return nil
	}
	token := p.codec.Encode(*next)
	return &token
}

// PageResponse is the uniform list body `{items, next_cursor}`; next_cursor is null on the last page.
type PageResponse[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}
