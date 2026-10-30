package preview

import (
	"context"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/config/validators"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/lookup"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
)

// PreviewValidationProfile runs the doc-only checks without the compile dry-run (the runner compiles).
func PreviewValidationProfile() config.ValidationProfile {
	return config.ValidationProfile{Stages: map[config.ValidationStage][]config.ValidatorName{
		config.StageDraft: {config.ValidatorSchema, config.ValidatorDeliveryMode},
	}}
}

// PipelineBuilder validates and compiles a draft into a pipeline for one run.
type PipelineBuilder struct {
	Compiler   engine.Compiler
	Validators *config.ValidatorManager
	Scripts    ScriptRuntimeProvider
	Salts      ctxhelpers.SaltProvider
	Registries codec.RegistryProvider
	Library    config.LibraryResolver
	// Sources is the live lookup gateway; runs wrap it in a PreviewGateway (mocks, capped timeout).
	Sources ctxhelpers.SourceGateway
}

// NewPipelineBuilder builds the validator manager with the preview profile.
func NewPipelineBuilder(compiler engine.Compiler, scripts ScriptRuntimeProvider, salts ctxhelpers.SaltProvider) (PipelineBuilder, error) {
	vm, err := validators.NewManager(PreviewValidationProfile(), validators.Deps{Compiler: compiler})
	if err != nil {
		return PipelineBuilder{}, err
	}
	return PipelineBuilder{Compiler: compiler, Validators: vm, Scripts: scripts, Salts: salts}, nil
}

// BuiltPipeline is a compiled draft with its branch layout and non-blocking issues.
type BuiltPipeline struct {
	Pipeline engine.Pipeline
	Layout   BranchLayout
	Issues   []IssueDTO
}

// Build validates (errors → InvalidConfigError) and compiles raw with the run's clock and scope.
func (b PipelineBuilder) Build(ctx context.Context, raw RawConfig, scope RunScope, clock ctxhelpers.Clock) (BuiltPipeline, error) {
	issues, err := b.Validators.Validate(ctx, config.StageDraft, scope.TeamID, raw)
	if err != nil {
		return BuiltPipeline{}, err
	}
	if issues.HasErrors() {
		return BuiltPipeline{}, &InvalidConfigError{Issues: issues}
	}
	doc, err := config.Parse(raw)
	if err != nil {
		return BuiltPipeline{}, &InvalidConfigError{Issues: config.Issues{config.NewError(config.IssueSchemaInvalid, config.PointerRoot, "invalid JSON: %v", err)}}
	}
	cfg, _ := doc.Config()
	scope.LookupPins = enginebind.ScopeLookups{}.Of(cfg)
	p, err := b.Compiler.Compile(ctx, doc, b.options(scope, clock))
	if ce, ok := err.(*engine.CompileError); ok {
		return BuiltPipeline{}, &InvalidConfigError{Issues: issues.Merge(ce.Issues)}
	}
	if err != nil {
		return BuiltPipeline{}, err
	}
	return BuiltPipeline{Pipeline: p, Layout: NewBranchLayout(cfg), Issues: IssueMapper{}.DTOs(issues)}, nil
}

func (b PipelineBuilder) options(scope RunScope, clock ctxhelpers.Clock) engine.CompileOptions {
	sources := lookup.NewPreviewGateway(b.Sources, scope.LookupMocks)
	helpers := ctxhelpers.Set{Clock: clock, Hash: ctxhelpers.NewSaltedHasher(b.Salts, scope.TeamID), Sources: sources}.WithDefaults()
	return engine.CompileOptions{Scripts: b.Scripts.Runtime(scope), Helpers: helpers, Meta: engine.RouteMeta{RouteID: string(PreviewRouteID), TeamID: scope.TeamID}, SchemaRegistries: b.Registries, Library: b.Library}
}

// RunPlan is everything one run needs: the draft, the records and who runs it.
type RunPlan struct {
	Config  RawConfig
	Records []SampleRecord
	Loaded  int
	Scope   RunScope
	Session Session
	Sink    *OutputCollector
}

// PreviewRunner compiles a draft and runs every record with tracing (design §5 run flow).
type PreviewRunner struct {
	Builder  PipelineBuilder
	Differ   PatchDiffer
	Deadline time.Duration
	Now      func() time.Time
}

// Run returns per-event results; hitting the deadline stops early with summary.incomplete.
func (r PreviewRunner) Run(ctx context.Context, plan RunPlan) (RunResultDTO, error) {
	start := r.Now()
	ctx, cancel := context.WithTimeout(ctx, r.Deadline)
	defer cancel()
	clock := &SampleClock{}
	built, err := r.Builder.Build(ctx, plan.Config, plan.Scope, clock)
	if err != nil {
		return RunResultDTO{}, err
	}
	defer built.Pipeline.Close(context.WithoutCancel(ctx))
	proc := EventProcessor{Pipeline: built.Pipeline, Clock: clock, Differ: r.Differ, Encoder: TraceEncoder{Layout: built.Layout}, Fallback: start, Sink: plan.Sink}
	warnings := NewWarningCollector()
	tally := NewSummaryTally(plan.Loaded)
	results := make([]EventResultDTO, 0, len(plan.Records))
	for _, rec := range plan.Records {
		res := proc.Process(ctx, rec)
		if ctx.Err() != nil {
			tally.Summary.Incomplete = true
			break
		}
		warnings.Add(res.Branches)
		tally.Add(res)
		results = append(results, res)
	}
	tally.Summary.DurationMs = r.Now().Sub(start).Milliseconds()
	return RunResultDTO{Summary: tally.Result(), Issues: append(built.Issues, warnings.Issues()...), Results: results}, nil
}

// EventProcessor runs one record: freeze clock, decode, process with trace, validate outputs.
type EventProcessor struct {
	Pipeline engine.Pipeline
	Clock    *SampleClock
	Differ   PatchDiffer
	Encoder  TraceEncoder
	Fallback time.Time
	Sink     *OutputCollector
}

// Process returns the event result; decode failures end the event as error/decode.
func (p EventProcessor) Process(ctx context.Context, rec SampleRecord) EventResultDTO {
	p.Clock.Set(p.frozenNow(rec))
	ev, err := p.Pipeline.Decode(ctx, rec.Raw())
	if err != nil {
		return EventResultDTO{Seq: rec.Seq, Status: StatusError, Error: ErrorDescriber{}.Describe(err, nil), Branches: []BranchResultDTO{}}
	}
	tr := NewPreviewTracer(p.Differ)
	outs, procErr := p.Pipeline.Process(ctx, ev, tr)
	validated := ValidatedOutputs{ByBranch: map[string][]OutputDTO{}}
	var outErr error
	if procErr == nil {
		validated, outErr = OutputValidator{}.Validate(ctx, p.Pipeline, outs)
	}
	hits := tr.Branches()
	branches := p.Encoder.Encode(hits, validated.ByBranch, procErr)
	res := EventResultDTO{Seq: rec.Seq, Branches: branches}
	if firstErr := p.firstError(procErr, outErr); firstErr != nil {
		res.Error = ErrorDescriber{}.Describe(firstErr, hits)
	}
	res.Status = p.Encoder.Classifier.Event(branches, res.Error)
	p.collect(res.Status, validated.Encoded)
	return res
}

// collect keeps the encoded outputs of passed events for a test-topic write.
func (p EventProcessor) collect(status EventStatus, encoded []engine.RawRecord) {
	if p.Sink == nil || status != StatusPassed {
		return
	}
	for _, r := range encoded {
		p.Sink.Add(r)
	}
}

// frozenNow is ctx.now for the event: its timestamp, else the run start.
func (p EventProcessor) frozenNow(rec SampleRecord) time.Time {
	if rec.Timestamp.IsZero() {
		return p.Fallback
	}
	return rec.Timestamp
}

func (EventProcessor) firstError(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// SummaryTally counts event statuses into the run summary, overall and per branch.
type SummaryTally struct {
	Summary  SummaryDTO
	branches map[string]*BranchSummaryDTO
	order    []string
}

// NewSummaryTally starts a tally for loaded events.
func NewSummaryTally(loaded int) *SummaryTally {
	return &SummaryTally{Summary: SummaryDTO{Loaded: loaded}, branches: map[string]*BranchSummaryDTO{}}
}

// Add counts one processed event and each of its branches.
func (t *SummaryTally) Add(res EventResultDTO) {
	t.Summary.Processed++
	StatusCounter{}.Count(res.Status, &t.Summary.Passed, &t.Summary.Filtered, &t.Summary.Dropped, &t.Summary.Errors)
	for _, b := range res.Branches {
		bs := t.branch(b.Branch)
		StatusCounter{}.Count(b.Status, &bs.Passed, &bs.Filtered, &bs.Dropped, &bs.Errors)
	}
}

// Result returns the summary with branch counts in first-seen (document) order.
func (t *SummaryTally) Result() SummaryDTO {
	out := t.Summary
	out.Branches = make([]BranchSummaryDTO, 0, len(t.order))
	for _, id := range t.order {
		out.Branches = append(out.Branches, *t.branches[id])
	}
	return out
}

func (t *SummaryTally) branch(id string) *BranchSummaryDTO {
	b, ok := t.branches[id]
	if !ok {
		b = &BranchSummaryDTO{Branch: id}
		t.branches[id], t.order = b, append(t.order, id)
	}
	return b
}

// StatusCounter increments the counter matching a status.
type StatusCounter struct{}

// Count adds one to passed, filtered, dropped or errors.
func (StatusCounter) Count(s EventStatus, passed, filtered, dropped, errors *int) {
	switch s {
	case StatusPassed:
		*passed++
	case StatusFiltered:
		*filtered++
	case StatusDropped:
		*dropped++
	default:
		*errors++
	}
}
