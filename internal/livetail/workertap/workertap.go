// Package workertap adapts monitoring's live-tail Tap to the worker runner's TailTap port
// (route-runtime RR-205 call site): runner events become TailRecords only once admitted.
package workertap

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/eventrouter/router/internal/livetail"
	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/engine"
)

// RunnerTap implements runner.TailTap over a livetail.Tap.
type RunnerTap struct {
	Tap livetail.Tap
}

// Active implements runner.TailTap.
func (t RunnerTap) Active(routeID string) bool {
	return t.Tap.Active(routeID)
}

// Offer implements runner.TailTap.
func (t RunnerTap) Offer(routeID string, ev runner.TailEvent) {
	t.Tap.Offer(routeID, EventSource{Event: ev})
}

// OutcomeTable maps runner outcomes to tail outcomes; failures that were not dead-lettered show as errors.
type OutcomeTable struct{}

// outcomes is the runner → tail outcome mapping.
var outcomes = map[domain.RecordOutcome]livetail.TailOutcome{
	domain.OutcomeProduced:     livetail.OutcomeSent,
	domain.OutcomeFiltered:     livetail.OutcomeFiltered,
	domain.OutcomeDropped:      livetail.OutcomeDropped,
	domain.OutcomeDeadLettered: livetail.OutcomeDeadLettered,
	domain.OutcomeSkipped:      livetail.OutcomeFiltered,
}

// Of returns the tail outcome of a finished record.
func (OutcomeTable) Of(o domain.RecordOutcome, failed bool) livetail.TailOutcome {
	out, ok := outcomes[o]
	if !ok || (failed && out != livetail.OutcomeDeadLettered) {
		return livetail.OutcomeError
	}
	return out
}

// EventSource builds the tail record of one runner event.
type EventSource struct {
	Event runner.TailEvent
}

// TailRecord implements livetail.RecordSource.
func (s EventSource) TailRecord(limits livetail.TailLimits) livetail.TailRecord {
	ev, codec := s.Event, PayloadCodec{MaxBytes: limits.MaxValueBytes}
	failed := ev.ErrorType != "" || ev.Error != ""
	rec := livetail.TailRecord{At: ev.At.UTC(), Partition: ev.Partition, Offset: ev.Offset, Branch: string(ev.Branch),
		Outcome: OutcomeTable{}.Of(ev.Outcome, failed), Step: ev.Step, ErrorType: ev.ErrorType, Error: ev.Error,
		Input: codec.Event(ev.Input, ""), Outputs: make([]livetail.TailEvent, 0, len(ev.Outputs))}
	for _, out := range ev.Outputs {
		rec.Outputs = append(rec.Outputs, codec.Output(out))
	}
	return rec
}

// PayloadCodec renders record bytes: JSON stays a tree, UTF-8 text stays text, anything else is base64;
// values over MaxBytes are cut and flagged.
type PayloadCodec struct {
	MaxBytes int
}

// Output renders one produced record.
func (c PayloadCodec) Output(o delivery.Outgoing) livetail.TailEvent {
	return c.Event(o.Record, o.Branch)
}

// Event renders one raw record.
func (c PayloadCodec) Event(r engine.RawRecord, branch engine.BranchID) livetail.TailEvent {
	value, enc, cut := c.Value(r.Value)
	ev := livetail.TailEvent{Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Branch: string(branch),
		Value: value, Encoding: enc, Truncated: cut}
	if r.Key != nil {
		ev.Key, _, _ = c.Value(r.Key)
	}
	for _, h := range r.Headers {
		ev.Headers = append(ev.Headers, livetail.Header{Key: h.Key, Value: string(h.Value)})
	}
	return ev
}

// Value decodes b into a JSON tree, text or base64.
func (c PayloadCodec) Value(b []byte) (any, livetail.Encoding, bool) {
	if c.MaxBytes > 0 && len(b) > c.MaxBytes {
		return strings.ToValidUTF8(string(b[:c.MaxBytes]), ""), livetail.EncodingText, true
	}
	var v any
	if len(b) > 0 && json.Unmarshal(b, &v) == nil {
		return v, livetail.EncodingJSON, false
	}
	if utf8.Valid(b) {
		return string(b), livetail.EncodingText, false
	}
	return base64.StdEncoding.EncodeToString(b), livetail.EncodingBase64, false
}
