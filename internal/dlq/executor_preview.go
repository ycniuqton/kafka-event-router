package dlq

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
)

// ServiceCaller names the preview caller of retries (no user session: the API acts for the route's team).
type ServiceCaller string

const RetryCaller ServiceCaller = "dlq-retry"

// PreviewExecutor runs retries in cmd/preview (sandbox isolation, ROUTER_DLQ_RETRY_EXECUTOR=preview):
// the records go to POST /internal/v1/preview/run as inline events with the active version config; the
// encoded outputs come back rendered and are produced here. Inline events carry JSON/text payloads only.
type PreviewExecutor struct {
	Preview  PreviewForwarder
	Producer DestinationProducer
}

// Kind implements RetryExecutor.
func (PreviewExecutor) Kind() ExecutorKind { return ExecutorPreview }

// Execute implements RetryExecutor in batches of PreviewBatch records.
func (e PreviewExecutor) Execute(ctx context.Context, target RetryTarget, recs []record.Record) []ItemResult {
	outputs, err := (RemoteOutputsOf{}).Version(target.Version)
	if err != nil {
		return ResultsOf{}.All(recs, ItemResult{Outcome: OutcomeFailed, ErrorType: FailCompile, Message: err.Error()})
	}
	run := &RetryRun{Producer: e.Producer, Broken: map[string]string{}}
	out := make([]ItemResult, 0, len(recs))
	for start := 0; start < len(recs); start += int(PreviewBatch) {
		batch := recs[start:min(start+int(PreviewBatch), len(recs))]
		out = append(out, e.batch(ctx, target, outputs, run, batch)...)
	}
	return out
}

// batch runs one preview call and turns each event result into an outcome, keeping input order;
// records the preview service cannot receive faithfully fail locally.
func (e PreviewExecutor) batch(ctx context.Context, target RetryTarget, outputs RemoteOutputs, run *RetryRun, recs []record.Record) []ItemResult {
	out, sendable, at := make([]ItemResult, len(recs)), []record.Record{}, []int{}
	for i, rec := range recs {
		if outputs.Undecodable(rec) {
			out[i] = ItemResult{Ref: rec.Ref, Outcome: OutcomeFailed, ErrorType: FailDecode, Message: ErrInlineDecode.Error()}
			continue
		}
		sendable, at = append(sendable, rec), append(at, i)
	}
	if len(sendable) == 0 {
		return out
	}
	results, err := e.call(ctx, target, sendable)
	bySeq := map[int]preview.EventResultDTO{}
	for _, r := range results.Results {
		bySeq[r.Seq] = r
	}
	for k, rec := range sendable {
		out[at[k]] = (RemoteResult{Outputs: outputs, Run: run}).Of(ctx, rec, bySeq[k], err)
	}
	return out
}

// call posts the run request; issues (compile errors) and transport failures fail the batch.
func (e PreviewExecutor) call(ctx context.Context, target RetryTarget, recs []record.Record) (preview.RunResultDTO, error) {
	events := make([]preview.InlineEvent, 0, len(recs))
	for _, r := range recs {
		events = append(events, (InlineOf{}).Event(r))
	}
	body, err := json.Marshal(preview.RunRequest{Events: events, Config: preview.RawConfig(target.Version.Config)})
	if err != nil {
		return preview.RunResultDTO{}, err
	}
	caller := preview.Caller{UserID: string(RetryCaller), TeamID: target.Route.TeamID}
	up, err := e.Preview.Forward(ctx, http.MethodPost, preview.PrefixInternal.Pattern(preview.EndpointRun), body, caller)
	if err != nil || !up.OK() {
		return preview.RunResultDTO{}, ErrStudioDown.Wrap(err)
	}
	var res preview.RunResultDTO
	dec := json.NewDecoder(bytes.NewReader(up.Body))
	dec.UseNumber()
	if err := dec.Decode(&res); err != nil {
		return preview.RunResultDTO{}, ErrStudioDown.Wrap(err)
	}
	if len(res.Issues) > 0 {
		return preview.RunResultDTO{}, &engine.CompileError{Issues: config.Issues{config.NewError(config.IssueSchemaInvalid, config.PointerRoot, "%s", res.Issues[0].Message)}}
	}
	return res, nil
}

// RemoteResult converts one remote event result into an outcome and delivers its outputs.
type RemoteResult struct {
	Outputs RemoteOutputs
	Run     *RetryRun
}

// Of returns failed (call or remote error), filtered (no outputs left after branch scope) or the delivery outcome.
func (r RemoteResult) Of(ctx context.Context, rec record.Record, res preview.EventResultDTO, callErr error) ItemResult {
	if callErr != nil {
		return ItemResult{Ref: rec.Ref, Outcome: OutcomeFailed, ErrorType: FailCompile, Message: callErr.Error()}
	}
	if res.Error != nil {
		kind := FailureKindOf[record.FromEngine(engine.ErrorType(res.Error.ErrorType))]
		return ItemResult{Ref: rec.Ref, Outcome: OutcomeFailed, ErrorType: kind, Message: res.Error.Message}
	}
	byConn := map[string][]engine.RawRecord{}
	for _, b := range res.Branches {
		if rec.Failure.WasDelivered(b.Branch) {
			continue
		}
		for _, o := range b.Outputs {
			raw, err := r.Outputs.Raw(b.Branch, o)
			if err != nil {
				return ItemResult{Ref: rec.Ref, Outcome: OutcomeFailed, ErrorType: FailOutputInvalid, Message: err.Error()}
			}
			conn := r.Outputs.Connections[b.Branch]
			byConn[conn] = append(byConn[conn], raw)
		}
	}
	if len(byConn) == 0 {
		return ItemResult{Ref: rec.Ref, Outcome: OutcomeFiltered}
	}
	return r.Run.deliver(ctx, rec.Ref, byConn)
}
