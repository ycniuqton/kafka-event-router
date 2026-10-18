// Package mock is the deterministic LLMProvider shipped with the product: a keyword intent parser
// for step proposals and a pattern-based explainer for JavaScript steps.
package mock

import (
	"context"
	"fmt"

	"github.com/eventrouter/router/internal/assist"
)

// ModelName is the model label the mock reports in completions.
type ModelName string

const MockModel ModelName = "mock-keyword-v1"

// taskHandler answers one assist task.
type taskHandler interface {
	Complete(p assist.Prompt) (assist.Completion, error)
}

// MockProvider implements assist.LLMProvider without any network call.
type MockProvider struct {
	tasks map[assist.AssistTask]taskHandler
}

// NewMockProvider wires the intent parser and the explainer by task.
func NewMockProvider() *MockProvider {
	return &MockProvider{tasks: map[assist.AssistTask]taskHandler{
		assist.TaskProposeSteps: proposeTask{parser: NewMockIntentParser()},
		assist.TaskExplain:      explainTask{explainer: NewMockExplainer()},
	}}
}

// Register adds the mock factory to the provider registry (called from cmd/api wiring).
func Register(reg *assist.ProviderRegistry) {
	reg.Register(assist.ProviderMock, func(assist.Settings) (assist.LLMProvider, error) { return NewMockProvider(), nil })
}

// Kind implements assist.LLMProvider.
func (*MockProvider) Kind() assist.ProviderKind { return assist.ProviderMock }

// Complete implements assist.LLMProvider.
func (m *MockProvider) Complete(ctx context.Context, p assist.Prompt) (assist.Completion, error) {
	if err := ctx.Err(); err != nil {
		return assist.Completion{}, err
	}
	h, ok := m.tasks[p.Task]
	if !ok {
		return assist.Completion{}, fmt.Errorf("mock provider: unsupported task %q", p.Task)
	}
	c, err := h.Complete(p)
	c.Model = string(MockModel)
	return c, err
}

type proposeTask struct {
	parser *MockIntentParser
}

func (t proposeTask) Complete(p assist.Prompt) (assist.Completion, error) {
	steps := t.parser.Parse(p.Instruction, NewFieldResolver(p.FieldSchema))
	raw, err := assist.StepCodec{}.Encode(steps)
	return assist.Completion{Steps: raw}, err
}

type explainTask struct {
	explainer *MockExplainer
}

func (t explainTask) Complete(p assist.Prompt) (assist.Completion, error) {
	steps, err := assist.StepCodec{}.Decode(p.Steps)
	if err != nil {
		return assist.Completion{}, err
	}
	code := ""
	if len(steps) > 0 && steps[0].Code != nil {
		code = *steps[0].Code
	}
	return assist.Completion{Text: t.explainer.Explain(code)}, nil
}
