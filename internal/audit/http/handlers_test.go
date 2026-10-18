package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
)

// fakeReader returns fixed entries and records the filter and page it was asked for.
type fakeReader struct {
	entries []audit.LogEntry
	next    *store.Cursor
	filter  audit.Filter
	page    audit.Page
}

func (f *fakeReader) Query(_ context.Context, filter audit.Filter, page audit.Page) ([]audit.LogEntry, *store.Cursor, error) {
	f.filter, f.page = filter, page
	return f.entries, f.next, nil
}

func newDeps(reader *fakeReader) Deps {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return Deps{Audit: audit.NewManager(reader, audit.NewCatalog()), Responder: api.NewResponder(problem.NewCatalog(), log),
		Pages: api.NewPageParser(store.NewCursorCodec("test-secret"))}
}

func TestListAuditLogPassesFiltersAndPages(t *testing.T) {
	reader := &fakeReader{entries: []audit.LogEntry{{ID: 7, At: time.Unix(10, 0), Action: audit.ActionSampleViewedRaw}},
		next: &store.Cursor{SortKey: []string{"x"}, ID: "7"}}
	h := NewModule(newDeps(reader)).Handlers()[0]
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-log?sensitive=true&q=orders&action=sample.viewed_raw&limit=500", nil)
	if err := h.Serve(rec, req); err != nil {
		t.Fatal(err)
	}
	if !reader.filter.SensitiveOnly || reader.filter.Text != "orders" || len(reader.filter.Actions) != 1 {
		t.Fatalf("filter not passed: %+v", reader.filter)
	}
	if reader.page.Limit != int(store.MaxPageLimit) {
		t.Fatalf("limit not clamped: %d", reader.page.Limit)
	}
	var body api.PageResponse[audit.LogEntry]
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].Category != audit.CategoryAccess || body.NextCursor == nil {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestListAuditLogRejectsUnknownAction(t *testing.T) {
	h := NewModule(newDeps(&fakeReader{})).Handlers()[0]
	err := h.Serve(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/audit-log?action=route.nuked", nil))
	var fe *audit.FilterError
	if !errors.As(err, &fe) || len(fe.Errors) != 1 {
		t.Fatalf("want filter error, got %v", err)
	}
}

func TestListAuditActionsReturnsCatalog(t *testing.T) {
	h := NewModule(newDeps(&fakeReader{})).Handlers()[1]
	rec := httptest.NewRecorder()
	if err := h.Serve(rec, httptest.NewRequest(http.MethodGet, string(PathAuditActions), nil)); err != nil {
		t.Fatal(err)
	}
	var body ActionList
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Items) < 10 {
		t.Fatalf("catalog not listed: %v %s", err, rec.Body.String())
	}
}
