package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/enginetest"
)

const lookupRoute = `{"apiVersion":"router/v1","name":"enrich","source":{"connectionId":"c","topics":["in"],"format":{"type":"json"}},
"branches":[{"id":"main","transform":{"steps":[{"kind":"lookup","id":"crm","lookup":{"sourceId":"src-crm",
 "request":{"kind":"http","path":"/customers/${value.customerId}"},"map":[{"from":"$.tier","to":"value.tier"}]}}]},
 "destination":{"connectionId":"c","topic":"out"}}]}`

func runLookup(t *testing.T, h *harness, mocks map[string]any) map[string]any {
	t.Helper()
	body := map[string]any{"config": json.RawMessage(lookupRoute), "events": []map[string]any{{"value": map[string]any{"customerId": "c1"}}}}
	if mocks != nil {
		body["lookupMocks"] = mocks
	}
	status, run := h.do(t, "u1", http.MethodPost, preview.EndpointRun, "", "", body)
	if status != http.StatusOK {
		t.Fatalf("run = %d %v", status, run)
	}
	return run
}

func stepOf(run map[string]any) (map[string]any, map[string]any) {
	ev := run["results"].([]any)[0].(map[string]any)
	branch := ev["branches"].([]any)[0].(map[string]any)
	out := branch["outputs"].([]any)[0].(map[string]any)
	return branch["steps"].([]any)[0].(map[string]any), out["value"].(map[string]any)
}

func TestPreviewRunsLiveLookups(t *testing.T) {
	gw := enginetest.NewFakeSourceGateway(func(ctxhelpers.SourceCall) enginetest.FakeAnswer {
		return enginetest.FakeAnswer{Found: true, Value: map[string]any{"tier": "gold"}}
	})
	h := newHarness(t, func(b *preview.PipelineBuilder) { b.Sources = gw })
	step, value := stepOf(runLookup(t, h, nil))
	logs := step["logs"].([]any)
	if value["tier"] != "gold" || len(logs) != 1 || !strings.HasPrefix(logs[0].(string), "lookup: mode=live source=src-crm result=hit") || len(gw.Calls()) != 1 {
		t.Fatalf("live lookup: value=%v logs=%v calls=%d", value, logs, len(gw.Calls()))
	}
}

func TestPreviewLookupMocksReplaceTheCall(t *testing.T) {
	gw := enginetest.NewFakeSourceGateway(func(ctxhelpers.SourceCall) enginetest.FakeAnswer { return enginetest.FakeAnswer{} })
	h := newHarness(t, func(b *preview.PipelineBuilder) { b.Sources = gw })
	step, value := stepOf(runLookup(t, h, map[string]any{"crm": map[string]any{"found": true, "value": map[string]any{"tier": "platinum"}}}))
	logs := step["logs"].([]any)
	if value["tier"] != "platinum" || !strings.HasPrefix(logs[0].(string), "lookup: mode=mock") || len(gw.Calls()) != 0 {
		t.Fatalf("mock lookup: value=%v logs=%v calls=%d", value, logs, len(gw.Calls()))
	}
}

func TestPreviewWithoutSourcesFailsTheStep(t *testing.T) {
	h := newHarness(t)
	run := runLookup(t, h, nil)
	ev := run["results"].([]any)[0].(map[string]any)
	if ev["status"] != "error" {
		t.Fatalf("want error status without a lookup gateway, got %v", ev)
	}
}
