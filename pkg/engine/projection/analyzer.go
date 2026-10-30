package projection

import (
	"strings"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/path"
)

// Inputs is what the analyser looks at for one compiled route.
type Inputs struct {
	Mode               engine.CodecMode
	SourceFormat       config.FormatType
	DestinationFormats []config.FormatType
	Access             *Recorder
}

// Plan is the codec decision of one route: the field tree when fast, else the reason.
type Plan struct {
	engine.CodecPlan
	Fields FieldTree
}

// Check is one condition a route must meet to run the fast codec.
type Check interface {
	Verify(in Inputs) (engine.CodecFallback, string, bool)
}

// Analyzer runs its checks in order; the first failing check names the fallback.
type Analyzer struct {
	Checks []Check
}

// DefaultAnalyzer checks mode, formats, scripts, component allow-lists and paths.
func DefaultAnalyzer() Analyzer {
	return Analyzer{Checks: []Check{
		ModeCheck{}, FormatCheck{}, ScriptCheck{}, FormulaCheck{},
		AllowList[config.StepKind]{Reason: engine.FallbackStepKind, Allowed: PathBoundedSteps, Used: func(r *Recorder) []config.StepKind { return r.Steps }},
		AllowList[config.NodeType]{Reason: engine.FallbackFilterNode, Allowed: PathBoundedNodes, Used: func(r *Recorder) []config.NodeType { return r.Nodes }},
		AllowList[config.ValueSourceKind]{Reason: engine.FallbackValueSource, Allowed: PathBoundedSources, Used: func(r *Recorder) []config.ValueSourceKind { return r.Sources }},
		WholeValueCheck{},
	}}
}

// Plan returns the codec plan of the route described by in.
func (a Analyzer) Plan(in Inputs) Plan {
	if in.Access == nil {
		return Plan{CodecPlan: engine.CodecPlan{Mode: engine.CodecTree, Fallback: engine.FallbackUnanalysed}}
	}
	for _, c := range a.Checks {
		if reason, detail, ok := c.Verify(in); !ok {
			return Plan{CodecPlan: engine.CodecPlan{Mode: engine.CodecTree, Fallback: reason, Detail: detail}}
		}
	}
	return Plan{CodecPlan: engine.CodecPlan{Mode: engine.CodecFast}, Fields: FieldTreeBuilder{}.Build(in.Access.Paths)}
}

// ModeCheck honours a configured tree mode.
type ModeCheck struct{}

func (ModeCheck) Verify(in Inputs) (engine.CodecFallback, string, bool) {
	return engine.FallbackModeTree, "", in.Mode.OrDefault() != engine.CodecTree
}

// FormatCheck requires JSON on the source value and on every destination value.
type FormatCheck struct{}

func (FormatCheck) Verify(in Inputs) (engine.CodecFallback, string, bool) {
	if in.SourceFormat != config.FormatTypeJSON {
		return engine.FallbackSourceFormat, string(in.SourceFormat), false
	}
	for _, f := range in.DestinationFormats {
		if f != config.FormatTypeJSON {
			return engine.FallbackDestinationFormat, string(f), false
		}
	}
	return engine.FallbackNone, "", true
}

// ScriptCheck rejects user code, whose field use cannot be known.
type ScriptCheck struct{}

func (ScriptCheck) Verify(in Inputs) (engine.CodecFallback, string, bool) {
	if len(in.Access.Scripts) > 0 {
		return engine.FallbackScript, in.Access.Scripts[0], false
	}
	return engine.FallbackNone, "", true
}

// FormulaCheck rejects formulas whose field references could not be listed.
type FormulaCheck struct{}

func (FormulaCheck) Verify(in Inputs) (engine.CodecFallback, string, bool) {
	if len(in.Access.Formulas) > 0 {
		return engine.FallbackFormula, in.Access.Formulas[0], false
	}
	return engine.FallbackNone, "", true
}

// WholeValueCheck rejects paths that address the whole value (or index into it).
type WholeValueCheck struct{}

func (WholeValueCheck) Verify(in Inputs) (engine.CodecFallback, string, bool) {
	for _, p := range in.Access.Paths {
		if p.Root == path.RootValue && (p.IsRoot() || p.Segments[0].IsIndex) {
			return engine.FallbackWholeValue, p.String(), false
		}
	}
	return engine.FallbackNone, "", true
}

// AllowList passes when every used component kind is known to touch only the paths it
// parses through the compile scope; anything else (new kinds included) falls back.
type AllowList[K ~string] struct {
	Reason  engine.CodecFallback
	Allowed map[K]bool
	Used    func(r *Recorder) []K
}

func (a AllowList[K]) Verify(in Inputs) (engine.CodecFallback, string, bool) {
	var bad []string
	for _, k := range a.Used(in.Access) {
		if !a.Allowed[k] {
			bad = append(bad, string(k))
		}
	}
	return a.Reason, strings.Join(bad, ","), len(bad) == 0
}

// PathBoundedSteps are the step kinds whose effect is confined to their parsed paths (and
// key / headers). keep_only, wrap, unwrap, map and js rebuild or read the whole value.
var PathBoundedSteps = map[config.StepKind]bool{
	config.StepKindRename: true, config.StepKindRemove: true, config.StepKindCopy: true, config.StepKindMove: true,
	config.StepKindSet: true, config.StepKindDefaultIfMissing: true, config.StepKindSetKey: true,
	config.StepKindMask: true, config.StepKindHash: true, config.StepKindConvertType: true, config.StepKindDropIf: true,
	config.StepKindLibrary: true, config.StepKindFlatten: true, config.StepKindRedactPattern: true,
	config.StepKindFormatDate: true, config.StepKindRound: true, config.StepKindStringCase: true,
	config.StepKindLookup: true, config.StepKindIfElse: true, config.StepKindSplitArray: true,
	config.StepKindHeaderAdd: true, config.StepKindHeaderRemove: true, config.StepKindHeaderRename: true,
	config.StepKindHeaderCopyToPayload: true, config.StepKindHeaderCopyFromPayload: true,
}

// PathBoundedNodes are the filter node types that only read their field.
var PathBoundedNodes = map[config.NodeType]bool{
	config.NodeTypeAnd: true, config.NodeTypeOr: true, config.NodeTypeNot: true, config.NodeTypeCond: true,
}

// PathBoundedSources are the value sources that read at most one parsed path or formula.
var PathBoundedSources = map[config.ValueSourceKind]bool{
	config.ValueSourceKindField: true, config.ValueSourceKindConstant: true,
	config.ValueSourceKindMetadata: true, config.ValueSourceKindFormula: true,
}
