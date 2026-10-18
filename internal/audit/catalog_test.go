package audit

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
)

func TestCatalogHook(t *testing.T) {
	h := NewCatalogHook(NewCatalog())
	e, err := h.Before(context.Background(), Entry{Action: ActionSampleViewedRaw})
	if err != nil || !e.Sensitive {
		t.Fatalf("raw sample view must be sensitive: %v %+v", err, e)
	}
	e, err = h.Before(context.Background(), Entry{Action: ActionRouteDeployed})
	if err != nil || e.Sensitive {
		t.Fatalf("deploy: %v %+v", err, e)
	}
	if _, err := h.Before(context.Background(), Entry{Action: "route.teleported"}); !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("unknown action: %v", err)
	}
	for _, a := range []Action{ActionAuthLogin, ActionAuthLoginFailed, ActionAuthLogout, ActionUserBootstrapAdmin, ActionDevSeed, ActionSecretRead} {
		if _, ok := NewCatalog().Describe(a); !ok {
			t.Errorf("platform action %s missing from catalog", a)
		}
	}
}

func TestFilterParser(t *testing.T) {
	p := NewFilterParser(NewCatalog())
	actor := uuid.New()
	q := url.Values{"actor": {actor.String()}, "action": {"route.deployed, sample.viewed_raw"}, "objectType": {"route"},
		"from": {"2026-10-01T00:00:00Z"}, "to": {"2026-10-13T00:00:00Z"}, "sensitive": {"true"}, "q": {"orders"}}
	f, err := p.Parse(q)
	if err != nil || *f.Actor != actor || len(f.Actions) != 2 || *f.ObjectType != "route" || !f.SensitiveOnly || f.Text != "orders" {
		t.Fatalf("parse: %v %+v", err, f)
	}
	bad := url.Values{"actor": {"x"}, "action": {"route.nope"}, "from": {"2026-10-13T00:00:00Z"}, "to": {"2026-10-01T00:00:00Z"},
		"sensitive": {"maybe"}, "q": {strings.Repeat("a", 201)}}
	_, err = p.Parse(bad)
	var fe *FilterError
	if !errors.As(err, &fe) || len(fe.Errors) != 5 {
		t.Fatalf("errors: %v", err)
	}
	paths := map[string]bool{}
	for _, e := range fe.Errors {
		paths[e.Path] = true
	}
	for _, want := range []string{"/actor", "/action", "/to", "/sensitive", "/q"} {
		if !paths[want] {
			t.Errorf("missing error at %s: %+v", want, fe.Errors)
		}
	}
}

func TestWhereBuilder(t *testing.T) {
	ot := ObjectType("route")
	where, args := WhereBuilder{}.Build(Filter{Actions: []Action{ActionRouteDeployed}, ObjectType: &ot, SensitiveOnly: true, Text: "50%_off"})
	got := strings.Join(where, " AND ")
	if got != `a.action = ANY($1) AND a.object_type = $2 AND a.sensitive AND (a.object_label ILIKE $3 OR a.object_id ILIKE $3 OR a.action ILIKE $3)` {
		t.Fatal(got)
	}
	if len(args) != 3 || args[2] != `%50\%\_off%` {
		t.Fatalf("args = %v", args)
	}
}

type fakeReader struct {
	page Page
}

func (r *fakeReader) Query(_ context.Context, _ Filter, page Page) ([]LogEntry, *store.Cursor, error) {
	r.page = page
	return []LogEntry{{ID: 1, Action: ActionUserUpdated}}, nil, nil
}

func TestManagerQuery(t *testing.T) {
	reader := &fakeReader{}
	m := NewManager(reader, NewCatalog())
	res, err := m.Query(context.Background(), Filter{}, Page{Limit: 5000})
	if err != nil || reader.page.Limit != 200 || res.Items[0].Category != CategoryAdmin {
		t.Fatalf("query: %v limit=%d %+v", err, reader.page.Limit, res.Items)
	}
	_, _ = m.Query(context.Background(), Filter{}, Page{})
	if reader.page.Limit != 50 {
		t.Fatalf("default limit = %d", reader.page.Limit)
	}
}
