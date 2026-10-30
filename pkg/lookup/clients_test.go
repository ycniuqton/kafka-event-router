package lookup_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/lookup"
)

func httpSpec(url string, auth lookup.HTTPAuth, secrets lookup.SecretBundle) lookup.SourceSpec {
	return lookup.SourceSpec{ID: "s1", Kind: lookup.KindHTTP, Limits: lookup.DefaultLimits(), Secrets: secrets,
		Settings: lookup.Settings{HTTP: &lookup.HTTPSettings{BaseURL: url + "/api", Auth: auth, Headers: map[string]string{"X-Default": "d"}}}}
}

func crmServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/customers/c1":
			if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("X-Default") != "d" || r.URL.Query().Get("t") != "acme" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"tier": "gold", "score": 7})
		case "/api/echo":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			_ = json.NewEncoder(w).Encode(map[string]any{"method": r.Method, "body": body})
		case "/api/busy":
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/api/bad":
			w.WriteHeader(http.StatusBadRequest)
		case "/api/text":
			_, _ = w.Write([]byte("plain text"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHTTPClientOutcomes(t *testing.T) {
	srv := crmServer(t)
	c, err := lookup.HTTPFactory{}.New(httpSpec(srv.URL, lookup.HTTPAuth{Mode: lookup.AuthBearer}, lookup.SecretBundle{Token: "tok"}))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	got, err := c.Fetch(ctx, ctxhelpers.SourceRequest{Path: "/customers/c1", Query: map[string]string{"t": "acme"}})
	if err != nil || !got.Found || !reflect.DeepEqual(got.Value, map[string]any{"tier": "gold", "score": int64(7)}) {
		t.Fatalf("hit: %+v %v", got, err)
	}
	if got, err := c.Fetch(ctx, ctxhelpers.SourceRequest{Path: "/customers/zz"}); err != nil || got.Found {
		t.Fatalf("404 must be a miss: %+v %v", got, err)
	}
	echo, err := c.Fetch(ctx, ctxhelpers.SourceRequest{Method: "POST", Path: "/echo", Body: map[string]any{"id": "c1"}})
	if err != nil || echo.Value.(map[string]any)["method"] != "POST" || echo.Value.(map[string]any)["body"].(map[string]any)["id"] != "c1" {
		t.Fatalf("post: %+v %v", echo, err)
	}
	if text, _ := c.Fetch(ctx, ctxhelpers.SourceRequest{Path: "/text"}); text.Value != "plain text" {
		t.Fatalf("text: %+v", text)
	}
	assertTransient(t, "503", c, ctxhelpers.SourceRequest{Path: "/busy"}, true)
	assertTransient(t, "400", c, ctxhelpers.SourceRequest{Path: "/bad"}, false)
}

func assertTransient(t *testing.T, name string, c lookup.Client, req ctxhelpers.SourceRequest, transient bool) {
	t.Helper()
	_, err := c.Fetch(context.Background(), req)
	var se *ctxhelpers.SourceError
	if !errors.As(err, &se) || se.Transient != transient {
		t.Fatalf("%s: want transient=%v, got %v", name, transient, err)
	}
}

func TestHTTPAuthModesAndProbe(t *testing.T) {
	var seen http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		if r.Header.Get("X-Api-Key") == "wrong" {
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer srv.Close()
	basic, _ := lookup.HTTPFactory{}.New(httpSpec(srv.URL, lookup.HTTPAuth{Mode: lookup.AuthBasic, Username: "u"}, lookup.SecretBundle{Password: "p"}))
	if err := basic.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if user, pass, ok := (&http.Request{Header: seen}).BasicAuth(); !ok || user != "u" || pass != "p" {
		t.Fatalf("basic auth not sent: %v", seen)
	}
	header, _ := lookup.HTTPFactory{}.New(httpSpec(srv.URL, lookup.HTTPAuth{Mode: lookup.AuthHeader, HeaderName: "X-Api-Key"}, lookup.SecretBundle{HeaderValue: "wrong"}))
	var pe *lookup.ProbeError
	if err := header.Probe(context.Background()); !errors.As(err, &pe) || pe.Stage != lookup.StageAuthenticated {
		t.Fatalf("403 must fail authenticated: %v", err)
	}
	down, _ := lookup.HTTPFactory{}.New(httpSpec("http://127.0.0.1:1", lookup.HTTPAuth{Mode: lookup.AuthNone}, lookup.SecretBundle{}))
	if err := down.Probe(context.Background()); !errors.As(err, &pe) || pe.Stage != lookup.StageReachable {
		t.Fatalf("refused must fail reachable: %v", err)
	}
	if _, err := (lookup.HTTPFactory{}).New(lookup.SourceSpec{Kind: lookup.KindHTTP, Settings: lookup.Settings{HTTP: &lookup.HTTPSettings{TLS: lookup.TLSSettings{CAPEM: "nope"}}}}); !errors.Is(err, lookup.ErrBadCA) {
		t.Fatalf("bad CA: %v", err)
	}
}

func TestRedisClient(t *testing.T) {
	mr := miniredis.RunT(t)
	_ = mr.Set("flag:new-ui", `{"on":true,"pct":20}`)
	_ = mr.Set("plain", "hello")
	mr.HSet("customer:c1", "tier", "gold")
	c := lookup.NewRedisClientWith(redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1}))
	defer c.Close()
	ctx := context.Background()
	cases := []struct {
		req   ctxhelpers.SourceRequest
		found bool
		value any
	}{
		{req: ctxhelpers.SourceRequest{Op: "get", Key: "flag:new-ui"}, found: true, value: map[string]any{"on": true, "pct": int64(20)}},
		{req: ctxhelpers.SourceRequest{Op: "get", Key: "plain"}, found: true, value: "hello"},
		{req: ctxhelpers.SourceRequest{Op: "get", Key: "none"}},
		{req: ctxhelpers.SourceRequest{Op: "hget", Key: "customer:c1", HashField: "tier"}, found: true, value: "gold"},
		{req: ctxhelpers.SourceRequest{Op: "mget", Keys: []string{"plain", "none"}}, found: true, value: []any{"hello", nil}},
		{req: ctxhelpers.SourceRequest{Op: "mget", Keys: []string{"none", "none2"}}, value: []any{nil, nil}},
	}
	for _, cs := range cases {
		got, err := c.Fetch(ctx, cs.req)
		if err != nil || got.Found != cs.found || (cs.value != nil && !reflect.DeepEqual(got.Value, cs.value)) {
			t.Fatalf("%+v: %+v %v", cs.req, got, err)
		}
	}
	if err := c.Probe(ctx); err != nil {
		t.Fatal(err)
	}
	mr.Close()
	assertTransient(t, "redis down", c, ctxhelpers.SourceRequest{Op: "get", Key: "x"}, true)
}

func TestRedisAuthProbe(t *testing.T) {
	mr := miniredis.RunT(t)
	mr.RequireAuth("secret")
	c, _ := lookup.RedisFactory{}.New(lookup.SourceSpec{Kind: lookup.KindRedis, Secrets: lookup.SecretBundle{Password: "wrong"},
		Settings: lookup.Settings{Redis: &lookup.RedisSettings{Addr: mr.Addr()}}})
	defer c.Close()
	var pe *lookup.ProbeError
	if err := c.Probe(context.Background()); !errors.As(err, &pe) || pe.Stage != lookup.StageAuthenticated {
		t.Fatalf("want authenticated failure, got %v", err)
	}
}

// fakePg is an in-memory PgRunner.
type fakePg struct {
	rows    map[string]map[string]any
	err     error
	pingErr error
	lastSQL string
	args    []any
}

func (f *fakePg) QueryRow(_ context.Context, sql string, args []any) (map[string]any, bool, error) {
	f.lastSQL, f.args = sql, args
	if f.err != nil {
		return nil, false, f.err
	}
	key, _ := args[0].(string)
	row, ok := f.rows[key]
	return row, ok, nil
}
func (f *fakePg) Ping(context.Context) error   { return f.pingErr }
func (f *fakePg) Sample(context.Context) error { return f.err }
func (f *fakePg) Close()                       {}

func TestPostgresClient(t *testing.T) {
	pg := &fakePg{rows: map[string]map[string]any{"s1": {"region": "eu"}}}
	c := lookup.NewPostgresClient(pg)
	got, err := c.Fetch(context.Background(), ctxhelpers.SourceRequest{SQL: "SELECT region FROM stores WHERE id = $1", Params: []any{"s1"}})
	if err != nil || !got.Found || got.Value.(map[string]any)["region"] != "eu" || pg.args[0] != "s1" {
		t.Fatalf("hit: %+v %v", got, err)
	}
	if got, _ := c.Fetch(context.Background(), ctxhelpers.SourceRequest{SQL: "x", Params: []any{"none"}}); got.Found {
		t.Fatal("no rows must be a miss")
	}
	pg.err = &pgconn.PgError{Code: "57014"}
	assertTransient(t, "statement timeout", c, ctxhelpers.SourceRequest{Params: []any{"s1"}}, true)
	pg.err = &pgconn.PgError{Code: "25006"}
	assertTransient(t, "read-only violation", c, ctxhelpers.SourceRequest{Params: []any{"s1"}}, false)
	pg.err, pg.pingErr = nil, &pgconn.PgError{Code: "28P01"}
	var pe *lookup.ProbeError
	if err := c.Probe(context.Background()); !errors.As(err, &pe) || pe.Stage != lookup.StageAuthenticated {
		t.Fatalf("auth probe: %v", err)
	}
}

func TestPostgresConfigIsReadOnly(t *testing.T) {
	spec := lookup.SourceSpec{Kind: lookup.KindPostgres, Limits: lookup.Limits{MaxConcurrency: 4}, Secrets: lookup.SecretBundle{Password: "p@ss"},
		Settings: lookup.Settings{Postgres: &lookup.PostgresSettings{Host: "db", Database: "ref", User: "reader", SSLMode: "disable", StatementTimeoutMs: 750}}}
	cfg, err := lookup.PostgresConfigBuilder{}.Build(spec)
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.ConnConfig.RuntimeParams
	if p["default_transaction_read_only"] != "on" || p["statement_timeout"] != "750" || cfg.MaxConns != 4 ||
		cfg.ConnConfig.Password != "p@ss" || cfg.ConnConfig.Port != 5432 || cfg.ConnConfig.Database != "ref" {
		t.Fatalf("config %+v %+v", p, cfg.ConnConfig)
	}
}

type fakeTables struct {
	rows config.LookupRows
	err  error
}

func (f fakeTables) LookupTable(context.Context, string, int) (config.LookupRows, error) {
	return f.rows, f.err
}

func TestStaticClient(t *testing.T) {
	spec := lookup.SourceSpec{Kind: lookup.KindStatic, Settings: lookup.Settings{Static: &lookup.StaticSettings{TableRef: "prices", Version: 2}}}
	c, _ := lookup.StaticFactory{Tables: fakeTables{rows: config.LookupRows{{Key: "sku1", Value: json.Number("9.5")}}}}.New(spec)
	got, err := c.Fetch(context.Background(), ctxhelpers.SourceRequest{Key: "sku1"})
	if err != nil || !got.Found || got.Value != 9.5 {
		t.Fatalf("hit: %+v %v", got, err)
	}
	if got, _ := c.Fetch(context.Background(), ctxhelpers.SourceRequest{Key: "x"}); got.Found {
		t.Fatal("unknown key must be a miss")
	}
	missing, _ := lookup.StaticFactory{Tables: fakeTables{err: config.ErrLibraryRefMissing}}.New(spec)
	assertTransient(t, "missing table", missing, ctxhelpers.SourceRequest{Key: "x"}, false)
	empty, _ := lookup.StaticFactory{Tables: fakeTables{}}.New(spec)
	var pe *lookup.ProbeError
	if err := empty.Probe(context.Background()); !errors.As(err, &pe) || pe.Stage != lookup.StageSample {
		t.Fatalf("empty table probe: %v", err)
	}
}
