package explain

import (
	"context"
	"strings"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/internal/assist/describe"
	"github.com/eventrouter/router/pkg/config"
)

// RecipeExplainer explains recipe steps deterministically from their config.
type RecipeExplainer struct {
	describer *describe.DescriberManager
}

// NewRecipeExplainer uses the shared step describer.
func NewRecipeExplainer(d *describe.DescriberManager) RecipeExplainer {
	return RecipeExplainer{describer: d}
}

// Kinds implements ExplainStrategy (every recipe kind).
func (RecipeExplainer) Kinds() []config.StepKind {
	return append(append([]config.StepKind{}, assist.RecipeKinds...), config.StepKindDropIf, config.StepKindLibrary)
}

// Explain implements ExplainStrategy.
func (r RecipeExplainer) Explain(_ context.Context, s config.Step) (StepExplanation, error) {
	return StepExplanation{Summary: r.describer.Describe(s), Bullets: []string{}, Source: SourceRules}, nil
}

// CompletionFormat is the text protocol of explain completions: summary line, then "- " bullet lines.
type CompletionFormat string

const (
	FormatBullet  CompletionFormat = "- "
	FormatNewline CompletionFormat = "\n"
)

// JsExplainer explains JavaScript steps through the ProviderManager.
type JsExplainer struct {
	providers *assist.ProviderManager
}

// NewJsExplainer binds the provider manager.
func NewJsExplainer(p *assist.ProviderManager) JsExplainer {
	return JsExplainer{providers: p}
}

// Kinds implements ExplainStrategy.
func (JsExplainer) Kinds() []config.StepKind { return []config.StepKind{config.StepKindJS} }

// Explain implements ExplainStrategy.
func (j JsExplainer) Explain(ctx context.Context, s config.Step) (StepExplanation, error) {
	raw, err := assist.StepCodec{}.Encode([]config.Step{s})
	if err != nil {
		return StepExplanation{}, err
	}
	c, err := j.providers.Complete(ctx, assist.Prompt{Task: assist.TaskExplain, Steps: raw})
	if err != nil {
		return StepExplanation{}, err
	}
	out := CompletionText{}.Parse(c.Text)
	out.Source = Source(j.providers.Kind())
	return out, nil
}

// CompletionText parses the explain text protocol.
type CompletionText struct{}

// Parse takes the first plain line as summary and every bullet line as a bullet.
func (CompletionText) Parse(text string) StepExplanation {
	out := StepExplanation{Bullets: []string{}}
	for _, line := range strings.Split(text, string(FormatNewline)) {
		line = strings.TrimSpace(line)
		bullet, isBullet := strings.CutPrefix(line, string(FormatBullet))
		switch {
		case line == "":
		case isBullet:
			out.Bullets = append(out.Bullets, bullet)
		case out.Summary == "":
			out.Summary = line
		}
	}
	return out
}
