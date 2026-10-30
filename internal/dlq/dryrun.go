package dlq

import (
	"context"
	"errors"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/pkg/engine"
)

// DryOutcome is what a replay would do with one record (nothing is produced, nothing resolved).
type DryOutcome string

const (
	DryWouldSend DryOutcome = "would_send"
	DryFiltered  DryOutcome = "filtered"
	DryFailed    DryOutcome = "failed"
)

// DryOutcomes lists outcomes in report order.
var DryOutcomes = []DryOutcome{DryWouldSend, DryFiltered, DryFailed}

// Workflow retry failure kinds of transient dependency failures (design 00001 §2.1).
const (
	FailLookup     RetryFailureKind = "lookup_unavailable"
	FailDependency RetryFailureKind = "dependency_unavailable"
)

// DependencyFailureKinds maps dependency kinds to failure kinds (other kinds: dependency_unavailable).
var DependencyFailureKinds = map[engine.DependencyKind]RetryFailureKind{engine.DependencyLookup: FailLookup}

// OutputView is one output a replay would produce.
type OutputView struct {
	Branch       string          `json:"branch"`
	ConnectionID string          `json:"connectionId"`
	Topic        string          `json:"topic"`
	Key          *EncodedValue   `json:"key"`
	Value        EncodedValue    `json:"value"`
	Headers      []EncodedHeader `json:"headers"`
}

// DryRunResult is the dry-run outcome of one record.
type DryRunResult struct {
	ID        string           `json:"id"`
	Ref       Ref              `json:"-"`
	Outcome   DryOutcome       `json:"outcome"`
	ErrorType RetryFailureKind `json:"errorType,omitempty"`
	Message   string           `json:"message,omitempty"`
	Outputs   []OutputView     `json:"outputs"`
}

// DryRunner runs records through a version without producing: an optional capability of a RetryExecutor.
type DryRunner interface {
	DryRun(ctx context.Context, target RetryTarget, recs []record.Record) []DryRunResult
}

// DryResults builds dry-run results.
type DryResults struct{}

// Failed converts a failed item result.
func (DryResults) Failed(ref Ref, res ItemResult) DryRunResult {
	return DryRunResult{ID: RefCodec{}.Encode(ref), Ref: ref, Outcome: DryFailed, ErrorType: res.ErrorType, Message: res.Message, Outputs: []OutputView{}}
}

// Outputs returns would_send with views of raws, or filtered when there are none.
func (DryResults) Outputs(ref Ref, views []OutputView) DryRunResult {
	out := DryRunResult{ID: RefCodec{}.Encode(ref), Ref: ref, Outcome: DryWouldSend, Outputs: views}
	if len(views) == 0 {
		out.Outcome = DryFiltered
	}
	return out
}

// View renders one encoded output.
func (DryResults) View(branch, conn string, raw engine.RawRecord) OutputView {
	headers := make([]EncodedHeader, 0, len(raw.Headers))
	for _, h := range raw.Headers {
		headers = append(headers, EncodedHeader{Key: h.Key, Value: string(h.Value), Encoding: EncodingUTF8})
	}
	v := OutputView{Branch: branch, ConnectionID: conn, Topic: raw.Topic, Value: ValueEncoder{}.Encode(raw.Value), Headers: headers}
	if raw.Key != nil {
		k := ValueEncoder{}.Encode(raw.Key)
		v.Key = &k
	}
	return v
}

// DryRun implements DryRunner: decode → process → branch scope → encode, no produce.
func (e InProcessExecutor) DryRun(ctx context.Context, target RetryTarget, recs []record.Record) []DryRunResult {
	compiled, err := e.Pipelines.Get(ctx, target.Route, target.Version)
	out := make([]DryRunResult, 0, len(recs))
	for _, rec := range recs {
		if err != nil {
			out = append(out, DryResults{}.Failed(rec.Ref, ItemResult{ErrorType: FailCompile, Message: err.Error()}))
			continue
		}
		out = append(out, (DryRun{Compiled: compiled}).One(ctx, rec))
	}
	return out
}

// DryRun is one in-process dry run over a compiled version.
type DryRun struct {
	Compiled *CompiledVersion
}

// One dry-runs a single record.
func (d DryRun) One(ctx context.Context, rec record.Record) DryRunResult {
	p := d.Compiled.Pipeline
	ev, err := p.Decode(ctx, engine.RawRecord{Key: rec.Key, Value: rec.Value, Headers: rec.Headers,
		Topic: rec.Source.Topic, Partition: rec.Source.Partition, Offset: rec.Source.Offset, Timestamp: rec.Failure.FailedAt})
	if err != nil {
		return DryResults{}.Failed(rec.Ref, ProcessFailure{}.Result(rec.Ref, err))
	}
	outs, err := p.Process(ctx, ev, nil)
	if err != nil {
		return DryResults{}.Failed(rec.Ref, ProcessFailure{}.Result(rec.Ref, err))
	}
	views := []OutputView{}
	for _, o := range (BranchScope{}).Keep(rec.Failure, outs) {
		raw, err := p.Encode(ctx, o)
		if err != nil {
			return DryResults{}.Failed(rec.Ref, ProcessFailure{}.Result(rec.Ref, err))
		}
		views = append(views, DryResults{}.View(string(o.Branch), d.Compiled.Destinations[o.Branch], raw))
	}
	return DryResults{}.Outputs(rec.Ref, views)
}

// DryRun implements DryRunner through the preview run endpoint (the Studio's preview, no produce).
func (e PreviewExecutor) DryRun(ctx context.Context, target RetryTarget, recs []record.Record) []DryRunResult {
	outputs, err := (RemoteOutputsOf{}).Version(target.Version)
	if err != nil {
		return (DryResults{}).AllFailed(recs, ItemResult{ErrorType: FailCompile, Message: err.Error()})
	}
	results, err := e.call(ctx, target, recs)
	if err != nil {
		return (DryResults{}).AllFailed(recs, ItemResult{ErrorType: FailCompile, Message: err.Error()})
	}
	bySeq := map[int]preview.EventResultDTO{}
	for _, r := range results.Results {
		bySeq[r.Seq] = r
	}
	out := make([]DryRunResult, 0, len(recs))
	for i, rec := range recs {
		out = append(out, (RemoteDryResult{Outputs: outputs}).Of(rec, bySeq[i]))
	}
	return out
}

// AllFailed fails every record with tmpl.
func (DryResults) AllFailed(recs []record.Record, tmpl ItemResult) []DryRunResult {
	out := make([]DryRunResult, 0, len(recs))
	for _, r := range recs {
		out = append(out, DryResults{}.Failed(r.Ref, tmpl))
	}
	return out
}

// RemoteDryResult converts one preview event result into a dry-run result.
type RemoteDryResult struct {
	Outputs RemoteOutputs
}

// Of returns failed (remote error, invalid output) or the would-be outputs of undelivered branches.
func (r RemoteDryResult) Of(rec record.Record, res preview.EventResultDTO) DryRunResult {
	if res.Error != nil {
		kind := FailureKindOf[record.FromEngine(engine.ErrorType(res.Error.ErrorType))]
		return DryResults{}.Failed(rec.Ref, ItemResult{ErrorType: kind, Message: res.Error.Message})
	}
	views := []OutputView{}
	for _, b := range res.Branches {
		if rec.Failure.WasDelivered(b.Branch) {
			continue
		}
		for _, o := range b.Outputs {
			raw, err := r.Outputs.Raw(b.Branch, o)
			if err != nil {
				return DryResults{}.Failed(rec.Ref, ItemResult{ErrorType: FailOutputInvalid, Message: err.Error()})
			}
			views = append(views, DryResults{}.View(b.Branch, r.Outputs.Connections[b.Branch], raw))
		}
	}
	return DryResults{}.Outputs(rec.Ref, views)
}

// DependencyKindOf returns the failure kind of a transient dependency error in err's chain.
type DependencyKindOf struct{}

// Of returns the kind and true when err carries an engine.DependencyError.
func (DependencyKindOf) Of(err error) (RetryFailureKind, bool) {
	de, ok := engine.AsDependencyError(err)
	if !ok {
		return "", false
	}
	if k, known := DependencyFailureKinds[de.Kind]; known {
		return k, true
	}
	return FailDependency, true
}

// errNoDryRun is returned when the configured executor cannot dry-run.
var errNoDryRun = errors.New("dlq: the configured retry executor cannot dry-run")
