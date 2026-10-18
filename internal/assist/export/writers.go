package export

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/path"
)

// WriterMessage holds the texts of per-member export failures.
type WriterMessage string

const (
	MsgPathUnsupported   WriterMessage = "Only value.* and key paths can be exported: %v"
	MsgSourceUnsupported WriterMessage = "Only field and constant sources can be exported."
	MsgStyleUnsupported  WriterMessage = "This mask style cannot be exported."
	MsgValueUnsupported  WriterMessage = "This value cannot be written as JavaScript: %v"
	MsgMissingMember     WriterMessage = "This member is required."
)

// WriteError points an export failure at a step member (At is relative to the step).
type WriteError struct {
	At      config.Pointer
	Message string
}

// MemberError builds a failure at one step member.
func MemberError(member config.PointerToken, msg string) *WriteError {
	return &WriteError{At: config.PointerRoot.Token(member), Message: msg}
}

// Issue converts the failure into a step_unsupported issue under the step pointer.
func (e *WriteError) Issue(step config.Pointer) config.Issue {
	return config.NewError(config.IssueStepUnsupported, step.Join(e.At), "%s", e.Message)
}

// StepJsWriter writes one step kind as a JavaScript statement block.
type StepJsWriter interface {
	Kind() config.StepKind
	Write(s config.Step) (string, *WriteError)
}

// JsWriterManager is the registry of writers by step kind.
type JsWriterManager struct {
	writers map[config.StepKind]StepJsWriter
}

// NewJsWriterManager registers the given writers.
func NewJsWriterManager(ws ...StepJsWriter) *JsWriterManager {
	m := &JsWriterManager{writers: map[config.StepKind]StepJsWriter{}}
	for _, w := range ws {
		m.writers[w.Kind()] = w
	}
	return m
}

// DefaultJsWriterManager covers the field, value, privacy and key recipes.
func DefaultJsWriterManager() *JsWriterManager {
	return NewJsWriterManager(
		relocateWriter{kind: config.StepKindRename}, relocateWriter{kind: config.StepKindMove},
		relocateWriter{kind: config.StepKindCopy, keepSource: true}, removeWriter{}, keepOnlyWriter{},
		setWriter{}, defaultWriter{}, setKeyWriter{}, maskWriter{}, hashWriter{},
	)
}

// Writer returns the writer of kind.
func (m *JsWriterManager) Writer(kind config.StepKind) (StepJsWriter, bool) {
	w, ok := m.writers[kind]
	return w, ok
}

// JsPath renders field paths as JavaScript segment arrays (["value","a",0]).
type JsPath struct{}

// Of parses p and rejects roots other than value and key.
func (JsPath) Of(p *string, member config.PointerToken) (string, *WriteError) {
	if p == nil {
		return "", MemberError(member, string(MsgMissingMember))
	}
	fp, err := path.Parser{}.Parse(*p)
	if err != nil || !(fp.Root == path.RootValue || (fp.Root == path.RootKey && fp.IsRoot())) {
		return "", MemberError(member, fmt.Sprintf(string(MsgPathUnsupported), *p))
	}
	segs := []any{string(fp.Root)}
	for _, s := range fp.Segments {
		segs = append(segs, JsPath{}.segment(s))
	}
	raw, _ := json.Marshal(segs)
	return string(raw), nil
}

func (JsPath) segment(s path.Segment) any {
	if s.IsIndex {
		return s.Index
	}
	return s.Name
}

// JsLiteral renders constants as JSON (valid JavaScript expressions).
type JsLiteral struct{}

// Of marshals v.
func (JsLiteral) Of(v any, member config.PointerToken) (string, *WriteError) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", MemberError(member, fmt.Sprintf(string(MsgValueUnsupported), err))
	}
	return string(raw), nil
}

// SourceExpr renders a value source as a statement that assigns `v` and runs `then` when the source resolves.
type SourceExpr struct{}

// SourceTemplate holds the source statement templates.
type SourceTemplate string

const (
	SourceConstant SourceTemplate = "{ const v = %s; %s }"
	SourceField    SourceTemplate = "{ const r = path.get(event, %s); if (r.ok) { const v = path.clone(r.v); %s } }"
)

// Wrap builds the statement for src around then.
func (SourceExpr) Wrap(src *config.ValueSource, then string) (string, *WriteError) {
	if src == nil {
		return "", MemberError(config.TokenSource, string(MsgMissingMember))
	}
	if src.From == config.ValueSourceKindConstant {
		lit, werr := JsLiteral{}.Of(src.Value, config.TokenSource)
		return fmt.Sprintf(string(SourceConstant), lit, then), werr
	}
	if src.From == config.ValueSourceKindField {
		p, werr := JsPath{}.Of(src.Path, config.TokenSource)
		return fmt.Sprintf(string(SourceField), p, then), werr
	}
	return "", MemberError(config.TokenSource, string(MsgSourceUnsupported))
}

// Joiner concatenates statements.
type Joiner struct{}

// Lines joins statements with a line break and indentation.
func (Joiner) Lines(lines []string) string {
	return strings.Join(lines, string(StatementBreak))
}

// List joins array items.
func (Joiner) List(items []string) string {
	return strings.Join(items, string(ListComma))
}
