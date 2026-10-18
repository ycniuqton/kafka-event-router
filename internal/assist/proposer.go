package assist

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/assist/describe"
	"github.com/eventrouter/router/pkg/config"
)

// ProposalRequest is what the Studio sends: the instruction, its current steps, the sample field paths
// and (already masked) sample events.
type ProposalRequest struct {
	Instruction  string
	CurrentSteps []config.Step
	Fields       []string
	Samples      []json.RawMessage
}

// Proposal is the validated answer: steps to append, one diff line per step, issues and the provider.
type Proposal struct {
	ID       string
	Steps    []config.Step
	Added    []string
	Issues   config.Issues
	Provider ProviderKind
}

// StepProposer turns an instruction into validated recipe steps through the ProviderManager.
type StepProposer struct {
	providers *ProviderManager
	policy    ProposalPolicy
	checker   *StepChecker
	describer *describe.DescriberManager
}

// NewStepProposer wires the proposer; the policy cap comes from the manager's limits.
func NewStepProposer(providers *ProviderManager, checker *StepChecker, describer *describe.DescriberManager) *StepProposer {
	policy := NewProposalPolicy(RecipeKinds, providers.Limits().MaxSteps)
	return &StepProposer{providers: providers, policy: policy, checker: checker, describer: describer}
}

// Propose validates the request, asks the provider and filters + validates its steps.
func (p *StepProposer) Propose(ctx context.Context, req ProposalRequest) (Proposal, error) {
	if err := p.Validate(req); err != nil {
		return Proposal{}, err
	}
	prompt, err := p.prompt(req)
	if err != nil {
		return Proposal{}, err
	}
	completion, err := p.providers.Complete(ctx, prompt)
	if err != nil {
		return Proposal{}, err
	}
	proposed, err := StepCodec{}.Decode(completion.Steps)
	if err != nil {
		return Proposal{}, ErrUnavailable.Wrap(err)
	}
	steps, issues := p.policy.Apply(proposed)
	return Proposal{
		ID: uuid.NewString(), Steps: steps, Added: p.describe(steps),
		Issues: issues.Merge(p.check(steps)).Sort(), Provider: p.providers.Kind(),
	}, nil
}

// Validate checks the request against the limits without calling the provider.
func (p *StepProposer) Validate(req ProposalRequest) error {
	limits := p.providers.Limits()
	switch {
	case strings.TrimSpace(req.Instruction) == "":
		return ErrInstructionEmpty
	case utf8.RuneCountInString(req.Instruction) > limits.MaxInstructionChars:
		return ErrInstructionTooLong
	case len(req.Fields) > limits.MaxFields:
		return ErrTooManyFields
	case len(req.CurrentSteps) > limits.MaxSteps:
		return ErrTooManySteps.At(string(PathCurrentSteps))
	}
	return nil
}

func (p *StepProposer) prompt(req ProposalRequest) (Prompt, error) {
	current, err := StepCodec{}.Encode(req.CurrentSteps)
	if err != nil {
		return Prompt{}, err
	}
	return Prompt{Task: TaskProposeSteps, Instruction: req.Instruction, FieldSchema: req.Fields, Samples: req.Samples, Steps: current}, nil
}

func (p *StepProposer) check(steps []config.Step) config.Issues {
	issues := config.Issues{}
	for i, s := range steps {
		issues = issues.Merge(p.checker.Check(config.Pointer(PathSteps).Index(i), s))
	}
	return issues
}

func (p *StepProposer) describe(steps []config.Step) []string {
	lines := make([]string, 0, len(steps))
	for _, s := range steps {
		lines = append(lines, p.describer.Describe(s))
	}
	return lines
}
