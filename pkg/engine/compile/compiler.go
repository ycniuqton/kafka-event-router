// Package compile turns a route document into an engine.Pipeline once per route
// version: codecs, filter tree, step chain, destination shaping and limits.
package compile

import (
	"context"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/compilectx"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/filter"
	"github.com/eventrouter/router/pkg/engine/filter/operators"
	"github.com/eventrouter/router/pkg/engine/formula"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/engine/step"
	"github.com/eventrouter/router/pkg/engine/step/recipes"
)

// MemoryUnit converts megabyte limits to bytes.
const MemoryUnit int64 = 1 << 20

// Registries are the extension points the compiler selects implementations from.
type Registries struct {
	Codecs    *codec.FactoryManager
	Nodes     *filter.NodeCompilerManager
	Operators *filter.OperatorManager
	Steps     *step.StepCompilerManager
	Values    *step.ValueSourceManager
	Upgraders *config.UpgraderManager
	Formulas  *formula.Compiler
}

// DefaultRegistries returns the built-in registries.
func DefaultRegistries() Registries {
	return Registries{
		Codecs:    codec.DefaultFactoryManager(),
		Nodes:     filter.DefaultNodeCompilerManager(),
		Operators: operators.Default(),
		Steps:     recipes.Default(),
		Values:    step.DefaultValueSourceManager(),
		Upgraders: config.DefaultUpgraderManager(),
		Formulas:  formula.NewDefaultCompiler(),
	}
}

// PipelineCompiler implements engine.Compiler.
type PipelineCompiler struct {
	reg Registries
}

// NewPipelineCompiler builds a compiler over the given registries.
func NewPipelineCompiler(reg Registries) *PipelineCompiler {
	return &PipelineCompiler{reg: reg}
}

// NewDefaultCompiler builds a compiler with the MVP registries.
func NewDefaultCompiler() *PipelineCompiler {
	return NewPipelineCompiler(DefaultRegistries())
}

// Compile builds the pipeline or returns *engine.CompileError with every issue found.
func (c *PipelineCompiler) Compile(ctx context.Context, doc *config.Document, opts engine.CompileOptions) (engine.Pipeline, error) {
	cfg, err := c.upgrade(doc)
	if err != nil {
		return nil, err
	}
	scope := c.newScope(ctx, cfg, opts)
	p := &compiledPipeline{meta: opts.Meta, helpers: scope.Helpers, modules: scope.Modules, limits: scope.Limits}
	p.source = SourceCodecs{}.Build(scope, c.reg.Codecs, cfg.Source)
	BranchIDs{}.Check(scope, cfg.Branches)
	for i, b := range cfg.Branches {
		p.branches = append(p.branches, c.branch(scope, cfg, i, b, p.source))
	}
	if scope.Failed() {
		_ = scope.Modules.Close(ctx)
		return nil, &engine.CompileError{Issues: scope.Issues.Sort()}
	}
	return p, nil
}

// upgrade selects the upgrader by apiVersion and decodes the typed config.
func (c *PipelineCompiler) upgrade(doc *config.Document) (*config.RouteConfig, error) {
	cfg, err := doc.Config()
	if err != nil {
		return nil, &engine.CompileError{Issues: config.Issues{config.NewError(config.IssueSchemaInvalid, config.PointerRoot, "%v", err)}}
	}
	up, err := c.reg.Upgraders.ForVersion(cfg.APIVersion)
	if err != nil {
		return nil, &engine.CompileError{Issues: config.Issues{config.NewError(config.IssueVersionUnsupported, config.PointerRoot.Token(config.TokenAPIVersion), "%v", err)}}
	}
	upgraded, err := up.Upgrade(doc)
	if err != nil {
		return nil, &engine.CompileError{Issues: config.Issues{config.NewError(config.IssueVersionUnsupported, config.PointerRoot, "%v", err)}}
	}
	return upgraded.Config()
}

// newScope builds the compile scope; route lookup tables are consulted before the injected store.
func (c *PipelineCompiler) newScope(ctx context.Context, cfg *config.RouteConfig, opts engine.CompileOptions) *compilectx.Scope {
	limits := cfg.Limits.Resolved()
	tables := ctxhelpers.NewTableLookupStore()
	helpers := opts.Helpers.WithDefaults()
	helpers.Lookup = ctxhelpers.LayeredLookupStore{Layers: []ctxhelpers.LookupStore{tables, helpers.Lookup}}
	return &compilectx.Scope{
		Ctx:     ctx,
		Scripts: opts.Scripts,
		ScriptLimits: script.Limits{
			Timeout:        time.Duration(limits.JSTimeoutMs) * time.Millisecond,
			MemoryBytes:    int64(limits.JSMemoryMb) * MemoryUnit,
			MaxOutputBytes: int64(limits.MaxOutputBytes),
		},
		Modules: &script.ModuleSet{},
		Library: compilectx.NewLibraryCache(opts.Library),
		Helpers: helpers,
		Limits:  limits,
		Tables:  tables,

		Registries: opts.SchemaRegistries,
	}
}

// branch compiles filter, steps and destination of branch i.
func (c *PipelineCompiler) branch(scope *compilectx.Scope, cfg *config.RouteConfig, i int, b config.Branch, src SourceCodecs) *branch {
	ptr := config.BranchPointer(i)
	out := &branch{id: engine.BranchID(b.ID), filterBase: ptr.Token(config.TokenFilter)}
	if b.Filter != nil {
		nc := &filter.NodeCompileCtx{Scope: scope, Base: ptr.Token(config.TokenFilter), Nodes: c.reg.Nodes, Operators: c.reg.Operators}
		out.filter, _ = nc.Compile(config.PointerRoot, *b.Filter)
	}
	out.chain = c.chain(scope, i, b)
	out.output = OutputShaperBuilder{}.Build(scope, c.reg.Codecs, ptr.Token(config.TokenDestination), b.Destination, src)
	return out
}

// chain compiles the enabled steps of branch i.
func (c *PipelineCompiler) chain(scope *compilectx.Scope, i int, b config.Branch) *step.StepChain {
	if b.Transform == nil {
		return step.NewStepChain(nil, scope.Limits.MaxOutputsPerEvent)
	}
	root := &step.StepCompileCtx{Scope: scope, Nodes: c.reg.Nodes, Operators: c.reg.Operators, Values: c.reg.Values,
		Formulas: c.reg.Formulas, Steps: c.reg.Steps}
	return root.Chain(config.BranchPointer(i).Token(config.TokenTransform).Token(config.TokenSteps), "", b.Transform.Steps)
}

// BranchIDs enforces unique branch ids, which tag outputs and trace entries.
type BranchIDs struct{}

// Check records SchemaInvalid at every repeated id.
func (BranchIDs) Check(scope *compilectx.Scope, branches []config.Branch) {
	seen := map[string]bool{}
	for i, b := range branches {
		if seen[b.ID] {
			scope.Fail(config.IssueSchemaInvalid, config.BranchPointer(i).Token(config.TokenID), "branch id %q is used more than once", b.ID)
		}
		seen[b.ID] = true
	}
}
