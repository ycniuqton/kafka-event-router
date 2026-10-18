// Package infer proposes steps that turn an input event into a pasted example output (transform by
// example): both trees are flattened, a configurable LeafMatcher chain pairs target leaves with input
// leaves, and a StepEmitter turns the pairs into rename/move/copy/set/remove recipe steps.
package infer

import (
	"encoding/json"
	"strings"

	"github.com/eventrouter/router/internal/platform/problem"
)

// Reason says why a target leaf got its source (also the matcher kind that found it).
type Reason string

const (
	ReasonValue    Reason = "value"
	ReasonName     Reason = "name"
	ReasonType     Reason = "type"
	ReasonConstant Reason = "constant"
)

// Confidence levels of the matchers (design §3.2).
type Confidence float64

const (
	ConfidenceExact     Confidence = 1.0
	ConfidenceAmbiguous Confidence = 0.8
	ConfidenceName      Confidence = 0.7
	ConfidenceConstant  Confidence = 0.5
	ConfidenceType      Confidence = 0.4
)

// JSONKind is the shape of a leaf value.
type JSONKind string

const (
	KindString JSONKind = "string"
	KindNumber JSONKind = "number"
	KindBool   JSONKind = "boolean"
	KindNull   JSONKind = "null"
	KindArray  JSONKind = "array"
	KindObject JSONKind = "object"
)

// PathRoot is the event member every proposed path starts from.
type PathRoot string

const RootValue PathRoot = "value"

// Separator joins path segments.
type Separator string

const Dot Separator = "."

// Leaf is one scalar (or array / empty object) of a flattened tree, addressed by its segments.
type Leaf struct {
	Segments []string
	Value    any
	Kind     JSONKind
}

// Path returns the event field path of the leaf (value.a.b).
func (l Leaf) Path() string {
	return string(RootValue) + string(Dot) + strings.Join(l.Segments, string(Dot))
}

// Name returns the last segment.
func (l Leaf) Name() string { return l.Segments[len(l.Segments)-1] }

// Parent returns the parent path segments joined ("" for top-level leaves).
func (l Leaf) Parent() string { return strings.Join(l.Segments[:len(l.Segments)-1], string(Dot)) }

// Match pairs a target leaf with its source leaf (nil for constants).
type Match struct {
	Target     Leaf
	Source     *Leaf
	Reason     Reason
	Confidence Confidence
}

// Mapping is one proposed target in the response.
type Mapping struct {
	Target     string     `json:"target"`
	Source     string     `json:"source,omitempty"`
	Reason     Reason     `json:"reason"`
	Confidence Confidence `json:"confidence"`
}

// Result is the inferred proposal: steps (route-schema steps), per-target mappings and dropped input paths.
type Result struct {
	Steps    []json.RawMessage `json:"steps"`
	Mappings []Mapping         `json:"mappings"`
	Unmapped []string          `json:"unmapped"`
}

// Request is the body of POST /preview/infer-mapping.
type Request struct {
	Input         json.RawMessage `json:"input"`
	DesiredOutput json.RawMessage `json:"desiredOutput"`
}

// Field names JSON Pointers of the request in problems.
type Field string

const (
	FieldInput   Field = "/input"
	FieldDesired Field = "/desiredOutput"
)

// Message holds user-facing texts.
type Message string

const (
	MsgDesiredNotObject Message = "Paste the desired output as a JSON object."
	MsgInputNotEvent    Message = "The input must be an event with an object value."
	MsgTooLarge         Message = "The example is too large to infer (at most 2,000 fields)."
)

// Limits bound one inference.
type Limits struct {
	MaxLeaves int
}

// DefaultLimits caps both trees at 2,000 leaves.
var DefaultLimits = Limits{MaxLeaves: 2000}

// Sentinel errors (422 with the request member's path).
var (
	ErrDesiredNotObject = problem.NewError(problem.KindValidation, string(MsgDesiredNotObject)).At(string(FieldDesired))
	ErrInputNotEvent    = problem.NewError(problem.KindValidation, string(MsgInputNotEvent)).At(string(FieldInput))
	ErrTooLarge         = problem.NewError(problem.KindValidation, string(MsgTooLarge)).At(string(FieldDesired))
)
