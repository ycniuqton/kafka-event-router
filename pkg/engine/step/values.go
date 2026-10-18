package step

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/formula"
)

// ValueResolver produces a value for set, set_key, header_add and mapper leaves;
// ok is false when the source is missing, err is a runtime failure (formula).
type ValueResolver interface {
	Resolve(sc *StepContext, ev *engine.Event) (v any, ok bool, err error)
}

// ValueSourceCompiler compiles one value source kind.
type ValueSourceCompiler interface {
	Kind() config.ValueSourceKind
	Compile(c *StepCompileCtx, ptr config.Pointer, src config.ValueSource) (ValueResolver, bool)
}

// ValueSourceManager selects value source compilers by kind.
type ValueSourceManager struct {
	compilers map[config.ValueSourceKind]ValueSourceCompiler
}

// NewValueSourceManager registers the given compilers.
func NewValueSourceManager(cs ...ValueSourceCompiler) *ValueSourceManager {
	m := &ValueSourceManager{compilers: map[config.ValueSourceKind]ValueSourceCompiler{}}
	for _, c := range cs {
		m.compilers[c.Kind()] = c
	}
	return m
}

// DefaultValueSourceManager registers field, constant, metadata and formula sources.
func DefaultValueSourceManager() *ValueSourceManager {
	return NewValueSourceManager(FieldSource{}, ConstantSource{}, MetadataSource{}, FormulaSource{})
}

// Compile dispatches by src.From.
func (m *ValueSourceManager) Compile(c *StepCompileCtx, ptr config.Pointer, src *config.ValueSource) (ValueResolver, bool) {
	if src == nil {
		c.Scope.Fail(config.IssueSchemaInvalid, ptr, "value source is required")
		return nil, false
	}
	vc, ok := m.compilers[src.From]
	if !ok {
		c.Scope.Fail(config.IssueFormulaInvalid, ptr.Token(config.TokenFrom), "value source %q is not supported yet", src.From)
		return nil, false
	}
	return vc.Compile(c, ptr, *src)
}

// FieldSource reads another field of the event.
type FieldSource struct{}

func (FieldSource) Kind() config.ValueSourceKind { return config.ValueSourceKindField }

func (FieldSource) Compile(c *StepCompileCtx, ptr config.Pointer, src config.ValueSource) (ValueResolver, bool) {
	if src.Path == nil {
		c.Scope.Fail(config.IssueSchemaInvalid, ptr, "field value source requires path")
		return nil, false
	}
	p, ok := c.Scope.Path(*src.Path, ptr.Token(config.TokenPath))
	return fieldResolver{ref: FieldRef{FieldPath: p}}, ok
}

type fieldResolver struct {
	ref FieldRef
}

func (r fieldResolver) Resolve(_ *StepContext, ev *engine.Event) (any, bool, error) {
	v, ok := r.ref.Get(ev)
	return engine.ValueCloner{}.Clone(v), ok, nil
}

// ConstantSource yields a constant from the document.
type ConstantSource struct{}

func (ConstantSource) Kind() config.ValueSourceKind { return config.ValueSourceKindConstant }

func (ConstantSource) Compile(_ *StepCompileCtx, _ config.Pointer, src config.ValueSource) (ValueResolver, bool) {
	return constantResolver{value: NumberNormalizer{}.Of(src.Value)}, true
}

type constantResolver struct {
	value any
}

func (r constantResolver) Resolve(*StepContext, *engine.Event) (any, bool, error) {
	return engine.ValueCloner{}.Clone(r.value), true, nil
}

// MetadataSource yields record or route metadata.
type MetadataSource struct{}

func (MetadataSource) Kind() config.ValueSourceKind { return config.ValueSourceKindMetadata }

// metadataReaders maps metadata names to readers.
var metadataReaders = map[config.MetadataName]func(sc *StepContext, ev *engine.Event) any{
	config.MetadataNameTopic:        func(_ *StepContext, ev *engine.Event) any { return ev.Topic },
	config.MetadataNamePartition:    func(_ *StepContext, ev *engine.Event) any { return int64(ev.Partition) },
	config.MetadataNameOffset:       func(_ *StepContext, ev *engine.Event) any { return ev.Offset },
	config.MetadataNameTimestamp:    func(_ *StepContext, ev *engine.Event) any { return TimeText{}.Of(ev.Timestamp) },
	config.MetadataNameRouteName:    func(sc *StepContext, _ *engine.Event) any { return sc.Meta.Name },
	config.MetadataNameRouteVersion: func(sc *StepContext, _ *engine.Event) any { return int64(sc.Meta.Version) },
}

func (MetadataSource) Compile(c *StepCompileCtx, ptr config.Pointer, src config.ValueSource) (ValueResolver, bool) {
	if src.Name == nil {
		c.Scope.Fail(config.IssueSchemaInvalid, ptr, "metadata value source requires name")
		return nil, false
	}
	read, ok := metadataReaders[*src.Name]
	if !ok {
		c.Scope.Fail(config.IssueSchemaInvalid, ptr.Token(config.TokenName), "unknown metadata %q", *src.Name)
		return nil, false
	}
	return metadataResolver{read: read}, true
}

type metadataResolver struct {
	read func(sc *StepContext, ev *engine.Event) any
}

func (r metadataResolver) Resolve(sc *StepContext, ev *engine.Event) (any, bool, error) {
	return r.read(sc, ev), true, nil
}

// FormulaSource evaluates a formula; a null result counts as a present null value.
type FormulaSource struct{}

func (FormulaSource) Kind() config.ValueSourceKind { return config.ValueSourceKindFormula }

func (FormulaSource) Compile(c *StepCompileCtx, ptr config.Pointer, src config.ValueSource) (ValueResolver, bool) {
	if src.Expr == nil {
		c.Scope.Fail(config.IssueSchemaInvalid, ptr, "formula value source requires expr")
		return nil, false
	}
	prog, ok := c.Formula(*src.Expr, ptr.Token(config.TokenExpr))
	return formulaResolver{program: prog}, ok
}

type formulaResolver struct {
	program *formula.Program
}

func (r formulaResolver) Resolve(sc *StepContext, ev *engine.Event) (any, bool, error) {
	v, err := r.program.Eval(sc.FormulaContext(ev))
	if err != nil {
		return nil, false, &StepError{Reason: ReasonFormulaFailed, Cause: err}
	}
	return v, true, nil
}
