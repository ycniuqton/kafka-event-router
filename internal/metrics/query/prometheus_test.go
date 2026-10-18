package query

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func promServer(t *testing.T, status int, body string, delay time.Duration) *PrometheusQuerier {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get(string(ParamQuery)) == "" {
			t.Errorf("missing query form: %v", err)
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	q, err := NewPrometheusQuerier(BackendConfig{URL: srv.URL + "/", Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestPrometheusInstantVector(t *testing.T) {
	q := promServer(t, http.StatusOK, `{"status":"success","data":{"resultType":"vector","result":[
		{"metric":{"route_id":"a"},"value":[1700000000.5,"12.5"]},{"metric":{"route_id":"b"},"value":[1700000000,"NaN"]}]}}`, 0)
	v, err := q.Instant(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 2 || v.ByLabel("route_id")["a"] != 12.5 || v.ByLabel("route_id")["b"] != 0 || v.Sum() != 12.5 {
		t.Fatalf("unexpected vector %+v", v)
	}
}

func TestPrometheusRangeMatrix(t *testing.T) {
	q := promServer(t, http.StatusOK, `{"status":"success","data":{"resultType":"matrix","result":[
		{"metric":{"partition":"0"},"values":[[1700000000,"1"],[1700000030,"2"]]}]}}`, 0)
	s, err := q.Range(context.Background(), "x", WindowEnding(time.Unix(1700000030, 0), 30*time.Second, 30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 || len(s[0].Points) != 2 || s[0].Points[1].Value != 2 || s[0].Points[0].At.Unix() != 1700000000 {
		t.Fatalf("unexpected matrix %+v", s)
	}
}

func TestPrometheusScalar(t *testing.T) {
	q := promServer(t, http.StatusOK, `{"status":"success","data":{"resultType":"scalar","result":[1700000000,"7"]}}`, 0)
	v, err := q.Instant(context.Background(), "x")
	if err != nil || v.Sum() != 7 {
		t.Fatalf("scalar: %v %v", v, err)
	}
}

func TestPrometheusFailuresAreUnavailable(t *testing.T) {
	cases := map[string]*PrometheusQuerier{
		"4xx":       promServer(t, http.StatusBadRequest, `{"status":"error","errorType":"bad_data","error":"parse error"}`, 0),
		"5xx":       promServer(t, http.StatusServiceUnavailable, `oops`, 0),
		"malformed": promServer(t, http.StatusOK, `{"status":"success","data":{"resultType":"vector","result":{}}}`, 0),
		"not json":  promServer(t, http.StatusOK, `<html>`, 0),
		"timeout":   promServer(t, http.StatusOK, `{"status":"success","data":{"resultType":"vector","result":[]}}`, time.Second),
	}
	for name, q := range cases {
		_, err := q.Instant(context.Background(), "x")
		if err == nil || !IsUnavailable(err) {
			t.Errorf("%s: want unavailable error, got %v", name, err)
		}
	}
	if _, err := NewPrometheusQuerier(BackendConfig{}); err == nil {
		t.Fatal("missing URL accepted")
	}
}

func TestQuerierManagerSelectsByKind(t *testing.T) {
	m := NewQuerierManager()
	q, err := m.Select(BackendMock, BackendConfig{})
	if err != nil || q.Kind() != BackendMock {
		t.Fatalf("mock: %v %v", q, err)
	}
	if _, err := m.Select("nope", BackendConfig{}); err == nil {
		t.Fatal("unknown backend accepted")
	}
}

func TestMockQuerierIsDeterministicPerRoute(t *testing.T) {
	now := time.Unix(1700000000, 0)
	m := NewMockQuerier(func() time.Time { return now })
	cat := ExprCatalog{}
	ctx := context.Background()
	a1, _ := m.Instant(ctx, cat.Build(QueryEventsIn, ForRoute("a", time.Hour, time.Minute)))
	a2, _ := m.Instant(ctx, cat.Build(QueryEventsIn, ForRoute("a", time.Hour, time.Minute)))
	b, _ := m.Instant(ctx, cat.Build(QueryEventsIn, ForRoute("b", time.Hour, time.Minute)))
	if a1.Sum() != a2.Sum() || a1.Sum() == b.Sum() {
		t.Fatalf("not deterministic per route: %v %v %v", a1, a2, b)
	}
	parts, _ := m.Range(ctx, cat.Build(QueryLagByPartition, ForRoute("a", time.Hour, time.Minute)), WindowEnding(now, time.Hour, 5*time.Minute))
	if len(parts) != DefaultMockShape.Partitions || len(parts[0].Points) != 13 {
		t.Fatalf("partitions %d points %d", len(parts), len(parts[0].Points))
	}
	byRoute, _ := m.Instant(ctx, cat.Build(QueryLagByRoute, Params{RouteIDs: []string{"a", "b"}}))
	if len(byRoute) != 2 || byRoute.ByLabel("route_id")["b"] == 0 {
		t.Fatalf("by route %+v", byRoute)
	}
}
