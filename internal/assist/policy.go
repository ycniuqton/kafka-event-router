package assist

import (
	"fmt"

	"github.com/eventrouter/router/pkg/config"
)

// RecipeKinds are the step kinds a proposal may contain besides JS (library refs and drop_if are never proposed).
var RecipeKinds = []config.StepKind{
	config.StepKindRename, config.StepKindCopy, config.StepKindMove, config.StepKindRemove, config.StepKindKeepOnly,
	config.StepKindSet, config.StepKindDefaultIfMissing, config.StepKindSetKey, config.StepKindMask, config.StepKindHash,
	config.StepKindConvertType,
}

// PolicyMessage holds the warning texts of the proposal policy.
type PolicyMessage string

const (
	MsgKindStripped PolicyMessage = "The assistant proposed an unsupported %q step; it was left out."
	MsgJSStripped   PolicyMessage = "The assistant proposed a JavaScript step although recipes cover the request; it was left out."
	MsgStepsCapped  PolicyMessage = "The assistant proposed %d steps; only the first %d are shown."
)

// ProposalPolicy keeps proposals small and readable: recipes first, unknown kinds out, a step cap.
type ProposalPolicy struct {
	allowed  map[config.StepKind]bool
	maxSteps int
}

// NewProposalPolicy builds the policy from the allowed recipe kinds and the step cap.
func NewProposalPolicy(kinds []config.StepKind, maxSteps int) ProposalPolicy {
	allowed := map[config.StepKind]bool{config.StepKindJS: true}
	for _, k := range kinds {
		allowed[k] = true
	}
	return ProposalPolicy{allowed: allowed, maxSteps: maxSteps}
}

// Apply returns the kept steps and a warning per removed step.
func (p ProposalPolicy) Apply(steps []config.Step) ([]config.Step, config.Issues) {
	issues := config.Issues{}
	known := []config.Step{}
	for _, s := range steps {
		if !p.allowed[s.Kind] {
			issues.Add(p.warning(MsgKindStripped, s.Kind))
			continue
		}
		known = append(known, s)
	}
	kept := p.preferRecipes(known, &issues)
	if len(kept) > p.maxSteps {
		issues.Add(config.NewWarning(config.IssueStepUnsupported, config.Pointer(PathSteps), string(MsgStepsCapped), len(kept), p.maxSteps))
		kept = kept[:p.maxSteps]
	}
	return kept, issues
}

// preferRecipes drops JS steps whenever the proposal also has recipe steps.
func (p ProposalPolicy) preferRecipes(steps []config.Step, issues *config.Issues) []config.Step {
	recipes := []config.Step{}
	for _, s := range steps {
		if s.Kind != config.StepKindJS {
			recipes = append(recipes, s)
		}
	}
	if len(recipes) == 0 || len(recipes) == len(steps) {
		return steps
	}
	issues.Add(config.NewWarning(config.IssueStepUnsupported, config.Pointer(PathSteps), string(MsgJSStripped)))
	return recipes
}

func (ProposalPolicy) warning(msg PolicyMessage, kind config.StepKind) config.Issue {
	return config.NewWarning(config.IssueStepUnsupported, config.Pointer(PathSteps), "%s", fmt.Sprintf(string(msg), kind))
}
