package testcases

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// RunTarget is the route identity and the full draft document a run compiles.
type RunTarget struct {
	TeamID    string
	RouteID   string
	RouteName string
	Document  json.RawMessage
}

// ScriptRuntimes supplies the JS runtime of a run (nil runtime: js steps fail to compile).
type ScriptRuntimes interface {
	Runtime(target RunTarget) script.Runtime
}

// NoScriptRuntimes disables js steps.
type NoScriptRuntimes struct{}

// Runtime implements ScriptRuntimes.
func (NoScriptRuntimes) Runtime(RunTarget) script.Runtime { return nil }

// RunnerDeps are the runner's collaborators; Salts may be nil (unsalted hashes), Library nil (library steps fail).
type RunnerDeps struct {
	Compiler    engine.Compiler
	Scripts     ScriptRuntimes
	Salts       ctxhelpers.SaltProvider
	Comparators *ComparatorManager
	Library     config.LibraryResolver
}

// TestCaseRunner compiles each branch of a draft once and runs the cases of that branch through it.
type TestCaseRunner struct {
	d RunnerDeps
}

// NewTestCaseRunner builds a runner.
func NewTestCaseRunner(d RunnerDeps) *TestCaseRunner {
	if d.Scripts == nil {
		d.Scripts = NoScriptRuntimes{}
	}
	if d.Salts == nil {
		d.Salts = ctxhelpers.StaticSaltProvider{}
	}
	return &TestCaseRunner{d: d}
}

// Run executes cases against target and reports every verdict in case order.
func (r *TestCaseRunner) Run(ctx context.Context, target RunTarget, cases []TestCase) RunReport {
	report := RunReport{Results: []Result{}}
	branches := &BranchPipelines{runner: r, target: target, compiled: map[string]CompiledBranch{}}
	defer branches.Close(ctx)
	for _, c := range cases {
		report.Add(r.runCase(ctx, branches.For(ctx, c.BranchID), c))
	}
	return report
}

// runCase processes one case's input and compares the observation with the expectation.
func (r *TestCaseRunner) runCase(ctx context.Context, b CompiledBranch, c TestCase) Result {
	res := Result{CaseID: c.ID, Name: c.Name, BranchID: c.BranchID, Diff: []trace.PatchOp{}, ActualOutput: json.RawMessage(nullJSON)}
	if b.Err != "" {
		return r.errored(res, OutcomeError, b.Err)
	}
	var in engine.Event
	if err := json.Unmarshal(c.InputEvent, &in); err != nil {
		return r.errored(res, OutcomeError, string(MsgInputUndecoded)+": "+err.Error())
	}
	actual, procErr := CaseExecution{}.Process(ctx, b.Pipeline, in)
	res.ActualOutcome, res.ActualOutput = actual.Outcome, actual.Output
	cmp := r.d.Comparators.Compare(Observation{Outcome: c.ExpectedOutcome, Output: c.ExpectedOutput}, actual)
	res.Diff, res.Error = cmp.Diff, procErr
	res.Result = ResultFail
	if cmp.Equal {
		res.Result = ResultPass
	}
	if res.Error == "" && !cmp.Equal {
		res.Error = cmp.Message
	}
	return res
}

// errored marks a case that could not run at all.
func (r *TestCaseRunner) errored(res Result, outcome Outcome, msg string) Result {
	res.Result, res.ActualOutcome, res.Error = ResultError, outcome, msg
	return res
}

// options builds the compile options of the target (route identity, team hash salt, JS runtime).
func (r *TestCaseRunner) options(target RunTarget) engine.CompileOptions {
	return engine.CompileOptions{
		Scripts: r.d.Scripts.Runtime(target),
		Library: r.d.Library,
		Helpers: ctxhelpers.Set{Hash: ctxhelpers.NewSaltedHasher(r.d.Salts, target.TeamID)}.WithDefaults(),
		Meta:    engine.RouteMeta{RouteID: target.RouteID, Name: target.RouteName, TeamID: target.TeamID},
	}
}

// nullJSON is the JSON null literal (no output).
const nullJSON = "null"

// CompiledBranch is one branch compiled alone, or the reason it could not be.
type CompiledBranch struct {
	Pipeline engine.Pipeline
	Err      string
}

// BranchPipelines compiles each branch of a run's document on first use.
type BranchPipelines struct {
	runner   *TestCaseRunner
	target   RunTarget
	compiled map[string]CompiledBranch
}

// For returns the compiled branch id.
func (b *BranchPipelines) For(ctx context.Context, id string) CompiledBranch {
	if c, ok := b.compiled[id]; ok {
		return c
	}
	c := b.compile(ctx, id)
	b.compiled[id] = c
	return c
}

func (b *BranchPipelines) compile(ctx context.Context, id string) CompiledBranch {
	raw, err := BranchDocument{}.Only(b.target.Document, id)
	if err != nil {
		return CompiledBranch{Err: err.Error()}
	}
	doc, err := config.Parse(raw)
	if err != nil {
		return CompiledBranch{Err: string(MsgCompileFailed) + ": " + err.Error()}
	}
	p, err := b.runner.d.Compiler.Compile(ctx, doc, b.runner.options(b.target))
	if err != nil {
		return CompiledBranch{Err: CompileMessage{}.Of(err)}
	}
	return CompiledBranch{Pipeline: p}
}

// Close releases every compiled pipeline.
func (b *BranchPipelines) Close(ctx context.Context) {
	for _, c := range b.compiled {
		if c.Pipeline != nil {
			_ = c.Pipeline.Close(ctx)
		}
	}
}

// CompileMessage renders a compile failure with the first issue's path.
type CompileMessage struct{}

// Of returns "The draft does not compile: <path>: <message>".
func (CompileMessage) Of(err error) string {
	var ce *engine.CompileError
	if errors.As(err, &ce) && len(ce.Issues) > 0 {
		return string(MsgCompileFailed) + ": " + string(ce.Issues[0].Path) + ": " + ce.Issues[0].Message
	}
	return string(MsgCompileFailed) + ": " + err.Error()
}
