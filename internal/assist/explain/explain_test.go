package explain_test

import (
	"context"
	"errors"
	"testing"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/internal/assist/describe"
	"github.com/eventrouter/router/internal/assist/explain"
	"github.com/eventrouter/router/internal/assist/mock"
	"github.com/eventrouter/router/pkg/config"
)

func str(s string) *string { return &s }

func manager() *explain.ExplainManager {
	d := describe.DefaultDescriberManager()
	providers := assist.NewProviderManager(mock.NewMockProvider(), assist.DefaultLimits())
	return explain.NewExplainManager(explain.NewRecipeExplainer(d), explain.NewJsExplainer(providers))
}

func TestRecipeChainIsRuleBased(t *testing.T) {
	style := config.MaskStyleLast4
	off := false
	e, err := manager().Explain(context.Background(), []config.Step{
		{Kind: config.StepKindRename, From: str("value.a"), To: str("value.b")},
		{Kind: config.StepKindMask, Field: str("value.card"), Style: &style},
		{Kind: config.StepKindRemove, Fields: []string{"value.x", "value.y"}, Enabled: &off},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Rename a → b", "Mask card (keep last 4)", "Remove x, y (disabled)"}
	if e.Source != explain.SourceRules || e.Summary != "Runs 3 steps in order." {
		t.Fatalf("unexpected %+v", e)
	}
	for i, w := range want {
		if e.Bullets[i] != w {
			t.Errorf("bullet %d = %q, want %q", i, e.Bullets[i], w)
		}
	}
}

func TestJsStepUsesProvider(t *testing.T) {
	e, err := manager().Explain(context.Background(), []config.Step{{Kind: config.StepKindJS,
		Code: str("function transform(event, ctx) { if (!event.value.ok) return null; ctx.log('x'); return event; }")}})
	if err != nil {
		t.Fatal(err)
	}
	if e.Source != explain.Source(assist.ProviderMock) || len(e.Bullets) != 2 || e.Summary == "" {
		t.Fatalf("unexpected %+v", e)
	}
}

func TestUnknownKind(t *testing.T) {
	_, err := manager().Explain(context.Background(), []config.Step{{Kind: "teleport"}})
	var unsupported explain.ErrKindUnsupported
	if !errors.As(err, &unsupported) {
		t.Fatalf("got %v", err)
	}
}

func TestCompletionTextProtocol(t *testing.T) {
	got := explain.CompletionText{}.Parse("\nSummary line\n- one\n- two\nignored extra\n")
	if got.Summary != "Summary line" || len(got.Bullets) != 2 {
		t.Fatalf("got %+v", got)
	}
}
