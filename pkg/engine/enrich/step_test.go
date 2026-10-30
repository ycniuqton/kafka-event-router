package enrich_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/enginetest"
	"github.com/eventrouter/router/pkg/engine/enrich"
	"github.com/eventrouter/router/pkg/engine/step"
	"github.com/eventrouter/router/pkg/engine/trace"
)

const httpLookup = `{"kind":"lookup","id":"crm","lookup":{"sourceId":"src-crm",
 "request":{"kind":"http","path":"/customers/${value.customerId}","query":{"t":"${headers.tenant}"}},
 "map":[{"from":"$.tier","to":"value.customer.tier"},{"from":"$.region","to":"value.customer.region"}]%s}}`

type lookupCase struct {
	name     string
	extra    string
	in       string
	answer   enginetest.FakeAnswer
	want     string
	dropped  bool
	wantType engine.ErrorType
	calls    int
}

func compileLookup(t *testing.T, extra string, gw ctxhelpers.SourceGateway) engine.Pipeline {
	t.Helper()
	steps := "[" + strings.Replace(httpLookup, "%s", extra, 1) + "]"
	opts := enginetest.GoldenOptions()
	opts.Helpers.Sources = gw
	return enginetest.MustCompileWith(t, enginetest.RouteJSON("null", steps), opts)
}

func runLookup(t *testing.T, c lookupCase) {
	t.Helper()
	gw := enginetest.NewFakeSourceGateway(func(ctxhelpers.SourceCall) enginetest.FakeAnswer { return c.answer })
	p := compileLookup(t, c.extra, gw)
	outs, err := p.Process(context.Background(), tenantEvent(t, p, c.in), nil)
	if len(gw.Calls()) != c.calls {
		t.Fatalf("%s: %d calls, want %d", c.name, len(gw.Calls()), c.calls)
	}
	if c.wantType != "" {
		var pe *engine.ProcessError
		if !errors.As(err, &pe) || pe.Type != c.wantType {
			t.Fatalf("%s: want %s, got %v", c.name, c.wantType, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("%s: %v", c.name, err)
	}
	if c.dropped {
		if len(outs) != 0 {
			t.Fatalf("%s: want dropped", c.name)
		}
		return
	}
	assertValue(t, c.name, outs[0].Value, c.want)
}

func tenantEvent(t *testing.T, p engine.Pipeline, value string) engine.Event {
	t.Helper()
	ev := enginetest.MustDecode(t, p, value)
	ev.Headers = []engine.Header{{Key: "tenant", Value: []byte("acme")}}
	return ev
}

func assertValue(t *testing.T, name string, got any, want string) {
	t.Helper()
	b, _ := json.Marshal(got)
	var g, w any
	_ = json.Unmarshal(b, &g)
	_ = json.Unmarshal([]byte(want), &w)
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("%s: got %s want %s", name, b, want)
	}
}

func TestSourceLookupOutcomes(t *testing.T) {
	gold := enginetest.FakeAnswer{Found: true, Value: map[string]any{"tier": "gold"}}
	transient := enginetest.FakeAnswer{Err: &ctxhelpers.SourceError{Transient: true, Reason: "503"}}
	permanent := enginetest.FakeAnswer{Err: &ctxhelpers.SourceError{Reason: "400"}}
	cases := []lookupCase{
		{name: "hit maps picks, missing pick leaves target", in: `{"customerId":"c1"}`, answer: gold, want: `{"customerId":"c1","customer":{"tier":"gold"}}`, calls: 1},
		{name: "miss skip (default)", in: `{"customerId":"c1"}`, answer: enginetest.FakeAnswer{}, want: `{"customerId":"c1"}`, calls: 1},
		{name: "miss default", extra: `,"onMiss":{"action":"default","value":"n/a"}`, in: `{"customerId":"c1"}`, want: `{"customerId":"c1","customer":{"tier":"n/a","region":"n/a"}}`, calls: 1},
		{name: "miss drop", extra: `,"onMiss":{"action":"drop"}`, in: `{"customerId":"c1"}`, dropped: true, calls: 1},
		{name: "miss fail", extra: `,"onMiss":{"action":"fail"}`, in: `{"customerId":"c1"}`, wantType: engine.ErrLookupMiss, calls: 1},
		{name: "missing key field is a miss without a call", extra: `,"onMiss":{"action":"default","value":0}`, in: `{"other":1}`, want: `{"other":1,"customer":{"tier":0,"region":0}}`},
		{name: "transient retried then lookup_unavailable", extra: `,"retries":2`, in: `{"customerId":"c1"}`, answer: transient, wantType: engine.ErrLookupUnavailable, calls: 3},
		{name: "permanent not retried, step error", in: `{"customerId":"c1"}`, answer: permanent, wantType: engine.ErrStep, calls: 1},
		{name: "error skip", extra: `,"onError":{"action":"skip"}`, in: `{"customerId":"c1"}`, answer: transient, want: `{"customerId":"c1"}`, calls: 2},
		{name: "error default", extra: `,"retries":0,"onError":{"action":"default","value":"unknown"}`, in: `{"customerId":"c1"}`, answer: transient, want: `{"customerId":"c1","customer":{"tier":"unknown","region":"unknown"}}`, calls: 1},
	}
	for _, c := range cases {
		runLookup(t, c)
	}
}

func TestSourceLookupRendersCall(t *testing.T) {
	gw := enginetest.NewFakeSourceGateway(func(ctxhelpers.SourceCall) enginetest.FakeAnswer {
		return enginetest.FakeAnswer{Found: true, Value: map[string]any{}}
	})
	p := compileLookup(t, "", gw)
	if _, err := p.Process(context.Background(), tenantEvent(t, p, `{"customerId":"c/1"}`), nil); err != nil {
		t.Fatal(err)
	}
	call := gw.Calls()[0]
	if call.SourceID != "src-crm" || call.StepID != "crm" || call.Kind != ctxhelpers.RequestHTTP ||
		call.Request.Method != "GET" || call.Request.Path != "/customers/c%2F1" || call.Request.Query["t"] != "acme" {
		t.Fatalf("call %+v", call)
	}
}

func TestSourceLookupTimeoutIsTransient(t *testing.T) {
	gw := enginetest.NewFakeSourceGateway(func(ctxhelpers.SourceCall) enginetest.FakeAnswer {
		time.Sleep(30 * time.Millisecond)
		return enginetest.FakeAnswer{Found: true}
	})
	p := compileLookup(t, `,"timeoutMs":5,"retries":0`, gw)
	_, err := p.Process(context.Background(), tenantEvent(t, p, `{"customerId":"c1"}`), nil)
	var pe *engine.ProcessError
	var lf *enrich.LookupFailure
	if !errors.As(err, &pe) || pe.Type != engine.ErrLookupUnavailable || !errors.As(err, &lf) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want lookup_unavailable timeout, got %v", err)
	}
	if de, ok := engine.AsDependencyError(err); !ok || de.Kind != engine.DependencyLookup || de.Name == "" {
		t.Fatalf("want a lookup DependencyError in the chain, got %v", err)
	}
}

func TestSourceLookupCacheAndLogLine(t *testing.T) {
	gw := enginetest.NewFakeSourceGateway(func(call ctxhelpers.SourceCall) enginetest.FakeAnswer {
		if strings.HasSuffix(call.Request.Path, "/c1") {
			return enginetest.FakeAnswer{Found: true, Value: map[string]any{"tier": "gold"}}
		}
		return enginetest.FakeAnswer{}
	})
	p := compileLookup(t, `,"cache":{"ttlMs":60000,"negativeTtlMs":60000,"maxEntries":10}`, gw)
	var modes []string
	for _, id := range []string{"c1", "c1", "c2", "c2", "c1"} {
		rec := trace.NewRecordingTracer(0)
		if _, err := p.Process(context.Background(), tenantEvent(t, p, `{"customerId":"`+id+`"}`), rec); err != nil {
			t.Fatal(err)
		}
		modes = append(modes, strings.Fields(rec.Trace().Branches[0].Steps[0].Logs[0])[1])
	}
	if strings.Join(modes, ",") != "mode=live,mode=cache,mode=live,mode=cache,mode=cache" {
		t.Fatalf("modes %v", modes)
	}
	if len(gw.Calls()) != 2 || gw.Observed(ctxhelpers.EventCacheHit) != 3 || gw.Observed(ctxhelpers.EventCacheMiss) != 2 {
		t.Fatalf("calls=%d hits=%d misses=%d", len(gw.Calls()), gw.Observed(ctxhelpers.EventCacheHit), gw.Observed(ctxhelpers.EventCacheMiss))
	}
	line := enrich.LookupLog{}.Line(ctxhelpers.AnswerCache, "src-crm", enrich.ResultHit, 0)
	if line != "lookup: mode=cache source=src-crm result=hit ms=0" {
		t.Fatalf("log line %q", line)
	}
}

func TestSourceLookupDeclaresFieldAccess(t *testing.T) {
	gw := enginetest.NewFakeSourceGateway(func(ctxhelpers.SourceCall) enginetest.FakeAnswer { return enginetest.FakeAnswer{} })
	_ = compileLookup(t, "", gw)
	var s step.Step = &enrich.SourceLookup{}
	if _, ok := s.(step.FieldAccessor); !ok {
		t.Fatal("SourceLookup must implement step.FieldAccessor")
	}
}

func TestNoGatewayIsPermanentStepError(t *testing.T) {
	steps := "[" + strings.Replace(httpLookup, "%s", "", 1) + "]"
	opts := enginetest.GoldenOptions()
	opts.Helpers.Sources = nil
	p := enginetest.MustCompileWith(t, enginetest.RouteJSON("null", steps), opts)
	_, err := p.Process(context.Background(), tenantEvent(t, p, `{"customerId":"c1"}`), nil)
	var pe *engine.ProcessError
	if !errors.As(err, &pe) || pe.Type != engine.ErrStep || !errors.Is(err, ctxhelpers.ErrSourceUnavailable) {
		t.Fatalf("want step error with ErrSourceUnavailable, got %v", err)
	}
}
