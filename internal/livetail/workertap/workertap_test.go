package workertap_test

import (
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/livetail"
	"github.com/eventrouter/router/internal/livetail/workertap"
	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/engine"
)

func TestOutcomeTable(t *testing.T) {
	cases := []struct {
		in     domain.RecordOutcome
		failed bool
		want   livetail.TailOutcome
	}{
		{domain.OutcomeProduced, false, livetail.OutcomeSent},
		{domain.OutcomeFiltered, false, livetail.OutcomeFiltered},
		{domain.OutcomeDropped, false, livetail.OutcomeDropped},
		{domain.OutcomeDropped, true, livetail.OutcomeError},
		{domain.OutcomeDeadLettered, true, livetail.OutcomeDeadLettered},
		{"unknown", false, livetail.OutcomeError},
	}
	for _, c := range cases {
		if got := (workertap.OutcomeTable{}).Of(c.in, c.failed); got != c.want {
			t.Errorf("%s/%v → %s, want %s", c.in, c.failed, got, c.want)
		}
	}
}

func TestEventSourceRendersPayloads(t *testing.T) {
	ev := runner.TailEvent{At: time.Unix(5, 0), Partition: 2, Offset: 9, Outcome: domain.OutcomeProduced,
		Input:   engine.RawRecord{Topic: "in", Key: []byte("k"), Value: []byte(`{"a":1}`), Headers: []engine.Header{{Key: "h", Value: []byte("v")}}},
		Outputs: []delivery.Outgoing{{Branch: "main", Record: engine.RawRecord{Topic: "out", Value: []byte{0xff, 0x00}}}}}
	limits := livetail.DefaultTailLimits()
	rec := workertap.EventSource{Event: ev}.TailRecord(limits)
	if rec.Outcome != livetail.OutcomeSent || rec.Input.Encoding != livetail.EncodingJSON || rec.Input.Key != "k" {
		t.Fatalf("input %+v", rec)
	}
	if rec.Outputs[0].Encoding != livetail.EncodingBase64 || rec.Outputs[0].Branch != "main" {
		t.Fatalf("output %+v", rec.Outputs[0])
	}
	big := workertap.PayloadCodec{MaxBytes: 4}
	if v, enc, cut := big.Value([]byte(strings.Repeat("x", 10))); v != "xxxx" || enc != livetail.EncodingText || !cut {
		t.Fatalf("truncate %v %v %v", v, enc, cut)
	}
}
