// Package explain writes plain-language summaries of transform steps: rule-based for recipe
// chains, through the LLM provider for JavaScript steps (design §3.2 StepExplainer).
package explain

import (
	"context"
	"fmt"

	"github.com/eventrouter/router/pkg/config"
)

// Source names who wrote an explanation: the rules or an LLM provider kind.
type Source string

const SourceRules Source = "rules"

// ChainPhrase is the summary template of a multi-step explanation.
type ChainPhrase string

const PhraseChain ChainPhrase = "Runs %d steps in order."

// StepExplanation is the explanation of one step.
type StepExplanation struct {
	Summary string
	Bullets []string
	Source  Source
}

// Explanation is the answer for one step or a chain of steps.
type Explanation struct {
	Summary string
	Bullets []string
	Source  Source
}

// ExplainStrategy explains the step kinds it declares.
type ExplainStrategy interface {
	Kinds() []config.StepKind
	Explain(ctx context.Context, s config.Step) (StepExplanation, error)
}

// ExplainManager picks the strategy by step kind; business logic talks only to it.
type ExplainManager struct {
	byKind map[config.StepKind]ExplainStrategy
}

// NewExplainManager registers strategies; later strategies win for a shared kind.
func NewExplainManager(strategies ...ExplainStrategy) *ExplainManager {
	m := &ExplainManager{byKind: map[config.StepKind]ExplainStrategy{}}
	for _, s := range strategies {
		for _, k := range s.Kinds() {
			m.byKind[k] = s
		}
	}
	return m
}

// Explain explains one step, or a chain with one bullet per step.
func (m *ExplainManager) Explain(ctx context.Context, steps []config.Step) (Explanation, error) {
	parts := make([]StepExplanation, 0, len(steps))
	for i, s := range steps {
		part, err := m.step(ctx, s)
		if err != nil {
			return Explanation{}, fmt.Errorf("step %d: %w", i, err)
		}
		parts = append(parts, part)
	}
	if len(parts) == 1 {
		return Explanation(parts[0]), nil
	}
	return m.chain(parts), nil
}

func (m *ExplainManager) step(ctx context.Context, s config.Step) (StepExplanation, error) {
	strategy, ok := m.byKind[s.Kind]
	if !ok {
		return StepExplanation{}, ErrKindUnsupported{Kind: s.Kind}
	}
	return strategy.Explain(ctx, s)
}

func (m *ExplainManager) chain(parts []StepExplanation) Explanation {
	out := Explanation{Summary: fmt.Sprintf(string(PhraseChain), len(parts)), Source: SourceRules, Bullets: []string{}}
	for _, p := range parts {
		out.Bullets = append(out.Bullets, p.Summary)
		if p.Source != SourceRules {
			out.Source = p.Source
		}
	}
	return out
}

// ErrKindUnsupported reports a step kind no strategy explains.
type ErrKindUnsupported struct {
	Kind config.StepKind
}

func (e ErrKindUnsupported) Error() string {
	return fmt.Sprintf("no explanation for step kind %q", e.Kind)
}
