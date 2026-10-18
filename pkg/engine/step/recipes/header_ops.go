package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/path"
	"github.com/eventrouter/router/pkg/engine/step"
)

// HeaderRef builds field refs of record headers by name (names may contain any character).
type HeaderRef struct{}

// Of returns the ref of header name, recording SchemaInvalid at member when absent.
func (HeaderRef) Of(c *step.StepCompileCtx, name *string, member config.PointerToken) (step.FieldRef, bool) {
	if name == nil {
		c.Fail(config.IssueSchemaInvalid, member, "%s is required", member)
		return step.FieldRef{}, false
	}
	return step.FieldRef{FieldPath: path.Parser{}.Build(path.RootHeaders, path.Segment{Name: *name})}, true
}

// HeaderAdd sets a header from a value source (replacing headers of the same name).
type HeaderAdd struct{}

func (HeaderAdd) Kind() config.StepKind { return config.StepKindHeaderAdd }
func (HeaderAdd) Group() step.Group     { return step.GroupKafka }

func (HeaderAdd) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	target, okH := HeaderRef{}.Of(c, raw.Header, config.TokenHeader)
	src, okS := c.Values.Compile(c, c.Pointer.Token(config.TokenSource), raw.Source)
	return &set{Base: step.NewBase(c, raw), target: target, source: src}, okH && okS
}

// HeaderRemove deletes every header of a name.
type HeaderRemove struct{}

func (HeaderRemove) Kind() config.StepKind { return config.StepKindHeaderRemove }
func (HeaderRemove) Group() step.Group     { return step.GroupKafka }

func (HeaderRemove) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	target, ok := HeaderRef{}.Of(c, raw.Header, config.TokenHeader)
	return &headerRemove{Base: step.NewBase(c, raw), target: target}, ok
}

type headerRemove struct {
	step.Base
	target step.FieldRef
}

func (s *headerRemove) Apply(_ *step.StepContext, in engine.Event) ([]engine.Event, error) {
	if !s.target.Delete(&in) {
		return step.One(in), s.Missing(s.target)
	}
	return step.One(in), nil
}

// headerEnd says which side of a header relocation is a header and which a field path.
type headerEnd string

const (
	endHeader headerEnd = "header"
	endField  headerEnd = "field"
)

// headerRelocateCompiler compiles rename and payload copies between headers and fields.
type headerRelocateCompiler struct {
	kind       config.StepKind
	from, to   headerEnd
	keepSource bool
}

// HeaderRename renames a header.
func HeaderRename() step.StepCompiler {
	return headerRelocateCompiler{kind: config.StepKindHeaderRename, from: endHeader, to: endHeader}
}

// HeaderCopyToPayload copies a header value into a field.
func HeaderCopyToPayload() step.StepCompiler {
	return headerRelocateCompiler{kind: config.StepKindHeaderCopyToPayload, from: endHeader, to: endField, keepSource: true}
}

// HeaderCopyFromPayload copies a field value into a header.
func HeaderCopyFromPayload() step.StepCompiler {
	return headerRelocateCompiler{kind: config.StepKindHeaderCopyFromPayload, from: endField, to: endHeader, keepSource: true}
}

func (h headerRelocateCompiler) Kind() config.StepKind { return h.kind }
func (headerRelocateCompiler) Group() step.Group       { return step.GroupKafka }

func (h headerRelocateCompiler) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	from, okF := h.end(c, raw, h.from, raw.Header, config.TokenHeader)
	toName := raw.NewHeader
	toToken := config.TokenNewHeader
	if h.from == endField {
		toName, toToken = raw.Header, config.TokenHeader
	}
	to, okT := h.end(c, raw, h.to, toName, toToken)
	return &relocate{Base: step.NewBase(c, raw), from: from, to: to, keepSource: h.keepSource}, okF && okT
}

// end resolves one side: a header by name or the step `field` path.
func (h headerRelocateCompiler) end(c *step.StepCompileCtx, raw config.Step, e headerEnd, name *string, member config.PointerToken) (step.FieldRef, bool) {
	if e == endField {
		return c.Path(raw.Field, config.TokenField)
	}
	return HeaderRef{}.Of(c, name, member)
}
