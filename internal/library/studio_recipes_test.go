package library_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/pkg/config"
)

// StudioRecipeFixture is written by web/src/domain/routeConfig/StepMapper.test.ts (one step per Studio recipe form).
const StudioRecipeFixture = "../../web/src/domain/routeConfig/testdata/studio_recipe_steps.json"

// TestStudioRecipeFormsAreSchemaValid (TS-27): every step the Studio's recipe forms serialise passes the route
// schema's step definition and the path grammar, so a form can never save a step the engine rejects.
func TestStudioRecipeFormsAreSchemaValid(t *testing.T) {
	raw, err := os.ReadFile(StudioRecipeFixture)
	if err != nil {
		t.Skipf("fixture not generated: %v", err)
	}
	var steps []config.Step
	if err := json.Unmarshal(raw, &steps); err != nil {
		t.Fatal(err)
	}
	checker, err := assist.NewStepChecker()
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) == 0 {
		t.Fatal("fixture has no steps")
	}
	for i, s := range steps {
		if issues := checker.Check(config.Pointer("/steps").Index(i), s); len(issues) > 0 {
			t.Errorf("step %d (%s): %v", i, s.Kind, issues)
		}
	}
}
