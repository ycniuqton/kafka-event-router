package testcases

import (
	"context"
	"testing"

	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/enginetest"
)

const sourceLookupSteps = `[{"kind":"lookup","id":"crm","lookup":{"sourceId":"src-crm",
 "request":{"kind":"http","path":"/customers/${value.customerId}"},"map":[{"from":"$.tier","to":"value.tier"}]}}]`

func TestRunnerServesSourceLookupsThroughTheGateway(t *testing.T) {
	gw := enginetest.NewFakeSourceGateway(func(ctxhelpers.SourceCall) enginetest.FakeAnswer {
		return enginetest.FakeAnswer{Found: true, Value: map[string]any{"tier": "gold"}}
	})
	r := NewTestCaseRunner(RunnerDeps{Compiler: compile.NewDefaultCompiler(), Comparators: DefaultComparatorManager(), Sources: gw})
	want := `{"key":null,"value":{"customerId":"c1","tier":"gold"},"headers":{}}`
	rep := r.Run(context.Background(), target("null", sourceLookupSteps), []TestCase{tc("enriched", `{"customerId":"c1"}`, OutcomePassed, want)})
	if rep.Passed != 1 || len(gw.Calls()) != 1 || gw.Calls()[0].SourceID != "src-crm" {
		t.Fatalf("report %+v calls %+v", rep, gw.Calls())
	}
}
