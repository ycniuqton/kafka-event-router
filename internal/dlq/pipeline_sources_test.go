package dlq

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/enginetest"
)

const sourceLookupSteps = `[{"kind":"lookup","id":"crm","lookup":{"sourceId":"src-crm",
 "request":{"kind":"http","path":"/customers/${value.customerId}"},"map":[{"from":"$.tier","to":"value.tier"}]}}]`

func TestPipelineCacheServesSourceLookupsForDryRunAndRetry(t *testing.T) {
	gw := enginetest.NewFakeSourceGateway(func(ctxhelpers.SourceCall) enginetest.FakeAnswer {
		return enginetest.FakeAnswer{Found: true, Value: map[string]any{"tier": "gold"}}
	})
	cache := NewPipelineCache(compile.NewDefaultCompiler(), NoScriptRuntimes{}, ctxhelpers.StaticSaltProvider{}).WithSources(gw)
	v := VersionConfig{ID: "v1", No: 1, Config: json.RawMessage(enginetest.RouteJSON("null", sourceLookupSteps))}
	compiled, err := cache.Get(context.Background(), RouteInfo{ID: "r1", TeamID: "t1"}, v)
	if err != nil {
		t.Fatal(err)
	}
	outs, err := compiled.Pipeline.Process(context.Background(), enginetest.MustDecode(t, compiled.Pipeline, `{"customerId":"c1"}`), nil)
	if err != nil || len(outs) != 1 || len(gw.Calls()) != 1 || gw.Calls()[0].TeamID != "t1" {
		t.Fatalf("outs %+v err %v calls %+v", outs, err, gw.Calls())
	}
	if got := outs[0].Value.(map[string]any)["tier"]; got != "gold" {
		t.Fatalf("tier %v", got)
	}
}

func TestDryRunReportsUnavailableLookupAsLookupUnavailable(t *testing.T) {
	gw := enginetest.NewFakeSourceGateway(func(ctxhelpers.SourceCall) enginetest.FakeAnswer {
		return enginetest.FakeAnswer{Err: &ctxhelpers.SourceError{Transient: true, Reason: "source unavailable", Cause: errors.New("503")}}
	})
	cache := NewPipelineCache(compile.NewDefaultCompiler(), NoScriptRuntimes{}, ctxhelpers.StaticSaltProvider{}).WithSources(gw)
	v := VersionConfig{ID: "v1", No: 1, Config: json.RawMessage(enginetest.RouteJSON("null", sourceLookupSteps))}
	rec := failure(0, record.ErrorLookupUnavailable)
	rec.Value = []byte(`{"customerId":"c1"}`)
	got := InProcessExecutor{Pipelines: cache}.DryRun(context.Background(), RetryTarget{Route: RouteInfo{ID: "r1", TeamID: "t1"}, Version: v}, []record.Record{rec})
	if len(got) != 1 || got[0].Outcome != DryFailed || got[0].ErrorType != FailLookup {
		t.Fatalf("dry run %+v", got)
	}
}
