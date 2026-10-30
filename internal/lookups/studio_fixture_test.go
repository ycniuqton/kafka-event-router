package lookups_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/enginetest"
)

// StudioLookupFixture is written by the web codec tests (one source-mode lookup step per request kind).
const StudioLookupFixture = "../../web/src/domain/routeConfig/testdata/lookup_source_steps.json"

// TestStudioLookupFormsCompile: every lookup step the Studio serialises passes the step schema and compiles.
func TestStudioLookupFormsCompile(t *testing.T) {
	raw, err := os.ReadFile(StudioLookupFixture)
	if err != nil {
		t.Skipf("fixture not generated: %v", err)
	}
	var steps []json.RawMessage
	if err := json.Unmarshal(raw, &steps); err != nil || len(steps) == 0 {
		t.Fatalf("fixture: %v (%d steps)", err, len(steps))
	}
	checker, err := assist.NewStepChecker()
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range steps {
		var step config.Step
		if err := json.Unmarshal(s, &step); err != nil {
			t.Fatal(err)
		}
		if issues := checker.Check(config.Pointer("/steps").Index(i), step); len(issues) > 0 {
			t.Errorf("step %d: schema %v", i, issues)
			continue
		}
		doc, _ := config.Parse([]byte(enginetest.RouteJSON("null", "["+string(s)+"]")))
		if _, err := compile.NewDefaultCompiler().Compile(context.Background(), doc, enginetest.GoldenOptions()); err != nil {
			t.Errorf("step %d: compile %v", i, err)
		}
	}
}
