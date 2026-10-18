package testcases

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// CaseExecution processes one input event and classifies what happened.
type CaseExecution struct{}

// Process runs in through p and shapes the outputs for their destinations (key policy, headers, codecs,
// size limit) exactly as preview and the runtime do; a processing or encoding error is the error outcome.
func (CaseExecution) Process(ctx context.Context, p engine.Pipeline, in engine.Event) (Observation, string) {
	tr := &StatusTracer{}
	outs, err := p.Process(ctx, in, tr)
	if err != nil {
		return Observation{Outcome: OutcomeError, Output: json.RawMessage(nullJSON)}, err.Error()
	}
	outcome := Outcome(tr.Status)
	if outcome == "" {
		outcome = OutcomePassed
	}
	if !outcome.CarriesOutput() {
		return Observation{Outcome: outcome, Output: json.RawMessage(nullJSON)}, ""
	}
	records, err := DestinationRecords{}.Of(ctx, p, outs)
	if err != nil {
		return Observation{Outcome: OutcomeError, Output: json.RawMessage(nullJSON)}, err.Error()
	}
	return Observation{Outcome: outcome, Output: OutputDocs{}.Of(records)}, ""
}

// DestinationRecords encodes outputs into the records their branch destinations would receive.
type DestinationRecords struct{}

// Of encodes every output; the first failure (output_invalid) is returned.
func (DestinationRecords) Of(ctx context.Context, p engine.Pipeline, outs []engine.Out) ([]engine.RawRecord, error) {
	records := make([]engine.RawRecord, 0, len(outs))
	for _, o := range outs {
		rec, err := p.Encode(ctx, o)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, nil
}

// StatusTracer keeps only the final event status (no snapshot cost).
type StatusTracer struct {
	trace.NoopTracer
	Status trace.EventStatus
}

// Finish implements trace.Tracer.
func (t *StatusTracer) Finish(status trace.EventStatus, _ *trace.ErrorInfo) { t.Status = status }

// OutputDoc is the locked shape of one output: key, value and headers as a name → text map.
type OutputDoc struct {
	Key     any               `json:"key"`
	Value   any               `json:"value"`
	Headers map[string]string `json:"headers"`
}

// OutputDocs renders outputs: one output is an object, several are an array, none is null.
type OutputDocs struct{}

// Of renders destination records.
func (OutputDocs) Of(records []engine.RawRecord) json.RawMessage {
	docs := make([]OutputDoc, 0, len(records))
	for _, r := range records {
		headers := map[string]string{}
		for _, h := range r.Headers {
			headers[h.Key] = string(h.Value)
		}
		docs = append(docs, OutputDoc{Key: RecordBytes{}.Render(r.Key), Value: RecordBytes{}.Render(r.Value), Headers: headers})
	}
	var v any = docs
	switch len(docs) {
	case 0:
		v = nil
	case 1:
		v = docs[0]
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(nullJSON)
	}
	return raw
}

// RecordBytes renders encoded key / value bytes the way preview shows them: JSON as JSON, UTF-8 text as a
// string, other bytes as base64, absent as null.
type RecordBytes struct{}

// Render returns the JSON-shaped form of b.
func (RecordBytes) Render(b []byte) any {
	switch {
	case b == nil:
		return nil
	case json.Valid(b):
		return json.RawMessage(append([]byte(nil), b...))
	case utf8.Valid(b):
		return string(b)
	default:
		return base64.StdEncoding.EncodeToString(b)
	}
}

// ErrBranchMissing reports a case whose branch is not in the draft.
var ErrBranchMissing = errors.New(string(MsgBranchMissing))

// BranchDocument narrows a route document to one branch, so the event status is that branch's.
type BranchDocument struct{}

// Only returns raw with `branches` reduced to the branch id.
func (BranchDocument) Only(raw json.RawMessage, id string) (json.RawMessage, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, errors.New(string(MsgDraftInvalid))
	}
	var branches []map[string]json.RawMessage
	if err := json.Unmarshal(doc[string(config.TokenBranches)], &branches); err != nil {
		return nil, errors.New(string(MsgDraftInvalid))
	}
	for _, b := range branches {
		var bid string
		if json.Unmarshal(b[string(config.TokenID)], &bid) == nil && bid == id {
			out, err := json.Marshal([]map[string]json.RawMessage{b})
			if err != nil {
				return nil, err
			}
			doc[string(config.TokenBranches)] = out
			return json.Marshal(doc)
		}
	}
	return nil, ErrBranchMissing
}
