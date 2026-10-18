package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
)

type body struct {
	Name   string `json:"name"`
	Config struct {
		Retries int `json:"retries"`
	} `json:"config"`
	Servers []string `json:"bootstrapServers"`
}

func decode(t *testing.T, raw string, limit BodyLimit) error {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(raw))
	_, err := Decoder[body]{Limit: limit}.Decode(httptest.NewRecorder(), r)
	return err
}

func TestDecoderJSONPointerErrors(t *testing.T) {
	cases := []struct {
		name, raw string
		limit     BodyLimit
		kind      problem.Kind
		path      string
	}{
		{"ok", `{"name":"a"}`, 0, "", ""},
		{"wrong nested type", `{"config":{"retries":"x"}}`, 0, problem.KindMalformedBody, "/config/retries"},
		{"unknown field", `{"nope":1}`, 0, problem.KindMalformedBody, "/nope"},
		{"syntax", `{"name":`, 0, problem.KindMalformedBody, ""},
		{"empty", ``, 0, problem.KindMalformedBody, ""},
		{"trailing", `{} {}`, 0, problem.KindMalformedBody, ""},
		{"too large", `{"name":"` + strings.Repeat("a", 100) + `"}`, 16, problem.KindPayloadTooLarge, ""},
	}
	catalog := problem.NewCatalog()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := decode(t, tc.raw, tc.limit)
			if tc.kind == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var c problem.Classified
			if !errors.As(err, &c) || c.ProblemKind() != tc.kind {
				t.Fatalf("got %v", err)
			}
			if d := catalog.Resolve(err); d.Path != tc.path {
				t.Fatalf("path = %q want %q", d.Path, tc.path)
			}
		})
	}
}

func TestCatalogMapping(t *testing.T) {
	c := problem.NewCatalog()
	cases := []struct {
		err    error
		status int
		typ    problem.TypeURI
	}{
		{NewValidationError().Add("/bootstrapServers/0", "host_port", "Use host:port."), 422, problem.TypeValidation},
		{store.ErrNotFound, 404, problem.TypeNotFound},
		{&store.PreconditionFailedError{Current: 8}, 412, problem.TypePreconditionFailed},
		{ErrPreconditionRequired, 428, problem.TypePreconditionRequired},
		{store.ErrInvalidCursor, 400, problem.TypeMalformedBody},
		{errors.New("boom secret=xyz"), 500, problem.TypeInternal},
	}
	for _, tc := range cases {
		d := c.Resolve(tc.err)
		if d.Status != tc.status || d.Type != tc.typ {
			t.Errorf("%v → %d %s", tc.err, d.Status, d.Type)
		}
	}
	v := c.Resolve(NewValidationError().Add("/a", "x", "m").Add("/b", "y", "n"))
	if v.Path != "/a" || len(v.Errors) != 2 {
		t.Errorf("validation details: %+v", v)
	}
	if d := c.Resolve(&store.PreconditionFailedError{Current: 8}); d.CurrentETag != `"8"` {
		t.Errorf("current_etag = %q", d.CurrentETag)
	}
	if d := c.Resolve(errors.New("boom secret=xyz")); strings.Contains(d.Detail, "secret") {
		t.Errorf("internal detail leaked: %q", d.Detail)
	}
}

func TestPageParser(t *testing.T) {
	codec := store.NewCursorCodec("k")
	p := NewPageParser(codec)
	parse := func(q string) (PageRequest, error) {
		return p.Parse(httptest.NewRequest(http.MethodGet, "/x?"+q, nil))
	}
	if r, err := parse(""); err != nil || r.Limit != 50 || r.After != nil {
		t.Fatalf("default: %+v %v", r, err)
	}
	if r, _ := parse("limit=1000"); r.Limit != 200 {
		t.Fatalf("clamp: %d", r.Limit)
	}
	for _, q := range []string{"limit=0", "limit=-1", "limit=abc"} {
		var v *ValidationError
		if _, err := parse(q); !errors.As(err, &v) {
			t.Errorf("%s: %v", q, err)
		}
	}
	token := codec.Encode(store.Cursor{SortKey: []string{"1"}, ID: "id"})
	if r, err := parse("cursor=" + token); err != nil || r.After.ID != "id" {
		t.Fatalf("cursor: %+v %v", r, err)
	}
	if _, err := parse("cursor=" + token + "x"); !errors.Is(err, store.ErrInvalidCursor) {
		t.Fatalf("tamper: %v", err)
	}
	out, _ := json.Marshal(PageResponse[int]{Items: []int{1}, NextCursor: p.Next(nil)})
	if string(out) != `{"items":[1],"next_cursor":null}` {
		t.Fatal(string(out))
	}
}

func TestPrecondition(t *testing.T) {
	req := func(h string) *http.Request {
		r := httptest.NewRequest(http.MethodPatch, "/x", nil)
		if h != "" {
			r.Header.Set("If-Match", h)
		}
		return r
	}
	if _, err := (Precondition{}).Require(req("")); !errors.Is(err, ErrPreconditionRequired) {
		t.Fatalf("missing: %v", err)
	}
	if v, err := (Precondition{}).Require(req(`"7"`)); err != nil || v != 7 {
		t.Fatalf("strong: %v %v", v, err)
	}
	if v, err := (Precondition{}).Require(req(`W/"9"`)); err != nil || v != 9 {
		t.Fatalf("weak: %v %v", v, err)
	}
	if _, err := (Precondition{}).Require(req(`7`)); !errors.Is(err, ErrBadIfMatch) {
		t.Fatalf("unquoted: %v", err)
	}
	err := (Precondition{}).Compare(7, 8)
	if d := problem.NewCatalog().Resolve(err); d.Status != 412 || d.CurrentETag != `"8"` {
		t.Fatalf("stale: %+v", d)
	}
}

type failing struct{}

func (failing) Route() RouteSpec {
	return RouteSpec{Method: http.MethodGet, Pattern: "/boom", Auth: AuthNone}
}
func (failing) Serve(http.ResponseWriter, *http.Request) error {
	return NewValidationError().Add("/name", "required", "Name is required.")
}

func TestMounterProblemAndRequestID(t *testing.T) {
	responder := NewResponder(problem.NewCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	m := NewMounter(responder, []GlobalMiddleware{RequestIDMiddleware{}, NoStoreMiddleware{}}, nil)
	router := newRouter()
	m.Install(router)
	m.Mount(router, Modules{failing{}})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))
	var d problem.Details
	_ = json.Unmarshal(rec.Body.Bytes(), &d)
	if rec.Code != 422 || d.RequestID == "" || d.RequestID != rec.Header().Get("X-Request-ID") || d.Path != "/name" {
		t.Fatalf("code %d details %+v", rec.Code, d)
	}
	if rec.Header().Get("Content-Type") != "application/problem+json" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers %v", rec.Header())
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if rec.Code != 404 {
		t.Fatalf("404 fallback: %d", rec.Code)
	}
}

// TestProblemLogsAbandonedRequestsAtDebug keeps client cancellations out of the error log.
func TestProblemLogsAbandonedRequestsAtDebug(t *testing.T) {
	var buf strings.Builder
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	responder := NewResponder(problem.NewCatalog(), log)
	live := httptest.NewRequest(http.MethodGet, "/x", nil)
	responder.Problem(httptest.NewRecorder(), live, errors.New("boom"))
	ctx, cancel := context.WithCancel(live.Context())
	cancel()
	responder.Problem(httptest.NewRecorder(), live.WithContext(ctx), errors.New("boom"))
	out := buf.String()
	if !strings.Contains(out, "level=ERROR msg=\""+string(LogRequestFailed)) || !strings.Contains(out, "level=DEBUG msg=\""+string(LogRequestAbandoned)) {
		t.Fatalf("log output: %s", out)
	}
}
