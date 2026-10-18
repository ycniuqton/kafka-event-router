package assist

import (
	"encoding/json"
	"time"

	"github.com/eventrouter/router/internal/platform/problem"
)

// ProviderKind names an LLMProvider implementation selected by ROUTER_ASSIST_PROVIDER.
type ProviderKind string

const (
	ProviderMock ProviderKind = "mock"
	// ProviderRules labels deterministic, provider-free work (infer mapping) in metrics.
	ProviderRules ProviderKind = "rules"
)

// AssistTask names what a prompt asks the provider to do.
type AssistTask string

const (
	TaskProposeSteps AssistTask = "propose_steps"
	TaskExplain      AssistTask = "explain"
	TaskInferMapping AssistTask = "infer_mapping"
)

// Prompt is the provider-agnostic request: instruction, field schema, at most a few samples and steps.
type Prompt struct {
	Task        AssistTask
	Instruction string
	FieldSchema []string
	Samples     []json.RawMessage
	Steps       json.RawMessage
}

// TokenUsage reports what a completion cost (zero for the mock provider).
type TokenUsage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
}

// Completion is a provider answer: free text (explain) and/or a JSON array of steps (propose).
type Completion struct {
	Text  string
	Steps json.RawMessage
	Model string
	Usage TokenUsage
}

// Limits bound every assist request (design §3.1 AssistLimits).
type Limits struct {
	MaxInstructionChars int
	MaxSamples          int
	MaxSampleBytes      int
	MaxSteps            int
	MaxFields           int
	Timeout             time.Duration
}

// LimitDefault holds the default limit values.
type LimitDefault int

const (
	DefaultMaxInstructionChars LimitDefault = 2000
	DefaultMaxSamples          LimitDefault = 2
	DefaultMaxSampleBytes      LimitDefault = 16 << 10
	DefaultMaxSteps            LimitDefault = 30
	DefaultMaxFields           LimitDefault = 500
)

// DefaultLimits returns the limits of design §3.1.
func DefaultLimits() Limits {
	return Limits{
		MaxInstructionChars: int(DefaultMaxInstructionChars), MaxSamples: int(DefaultMaxSamples),
		MaxSampleBytes: int(DefaultMaxSampleBytes), MaxSteps: int(DefaultMaxSteps), MaxFields: int(DefaultMaxFields),
		Timeout: time.Duration(DefaultTimeout),
	}
}

// RequestPath names JSON Pointers of assist request members used in problems.
type RequestPath string

const (
	PathInstruction  RequestPath = "/instruction"
	PathCurrentSteps RequestPath = "/currentSteps"
	PathFields       RequestPath = "/fields"
	PathSteps        RequestPath = "/steps"
	PathStep         RequestPath = "/step"
)

// Message holds user-facing texts of assist failures.
type Message string

const (
	MsgUnavailable        Message = "The assistant is unavailable right now. Your instruction is kept; retry in a moment."
	MsgInstructionEmpty   Message = "Describe the change you want, for example \"mask email\"."
	MsgInstructionTooLong Message = "The instruction is too long. Shorten it to at most 2000 characters."
	MsgTooManyFields      Message = "Too many fields were sent. Send at most 500 field paths."
	MsgNoSteps            Message = "Send at least one step."
	MsgTooManySteps       Message = "Too many steps were sent. Send at most 30 steps."
	MsgRateLimited        Message = "Too many assistant requests. Wait a minute and retry."
	MsgAssistOff          Message = "AI assist is turned off for this workspace. A workspace admin can turn it on in Settings → Workspace."
)

// Sentinel errors of the assistant; handlers turn them into RFC 7807 problems.
var (
	ErrUnavailable        = problem.NewError(problem.KindUnavailable, string(MsgUnavailable))
	ErrInstructionEmpty   = problem.NewError(problem.KindValidation, string(MsgInstructionEmpty)).At(string(PathInstruction))
	ErrInstructionTooLong = problem.NewError(problem.KindValidation, string(MsgInstructionTooLong)).At(string(PathInstruction))
	ErrTooManyFields      = problem.NewError(problem.KindValidation, string(MsgTooManyFields)).At(string(PathFields))
	ErrNoSteps            = problem.NewError(problem.KindValidation, string(MsgNoSteps)).At(string(PathSteps))
	ErrTooManySteps       = problem.NewError(problem.KindValidation, string(MsgTooManySteps)).At(string(PathSteps))
	ErrRateLimited        = problem.NewError(problem.KindRateLimited, string(MsgRateLimited))
	ErrAssistOff          = problem.NewError(problem.KindForbidden, string(MsgAssistOff))
)
