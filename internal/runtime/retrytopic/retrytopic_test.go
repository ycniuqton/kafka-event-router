package retrytopic

import (
	"context"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
)

func TestBackoffSchedule(t *testing.T) {
	s := BackoffSchedule{Base: time.Second, Max: 10 * time.Second}
	want := map[int]time.Duration{0: time.Second, 1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second, 4: 8 * time.Second, 5: 10 * time.Second, 10: 10 * time.Second}
	for n, d := range want {
		if got := s.Delay(n); got != d {
			t.Errorf("delay(%d) = %s, want %s", n, got, d)
		}
	}
	def := NewPlanner(&config.ErrorPolicy{Retry: &config.RetryPolicy{Enabled: true}}).Schedule()
	if def.Base != time.Second || def.Max != 5*time.Minute || def.Delay(20) != 5*time.Minute {
		t.Fatalf("defaults %+v", def)
	}
}

func TestPlannerDecisions(t *testing.T) {
	n := 3
	on := NewPlanner(&config.ErrorPolicy{Retry: &config.RetryPolicy{Enabled: true, MaxAttempts: &n}})
	off := NewPlanner(&config.ErrorPolicy{Retry: &config.RetryPolicy{Enabled: false}})
	if NewPlanner(nil).Enabled() || off.Diverts(domain.CategoryDependencyTransient) {
		t.Fatal("disabled policy diverts")
	}
	for _, c := range []domain.ErrorCategory{domain.CategoryStepTimeout, domain.CategoryDependencyTransient, domain.CategoryDestinationTransient} {
		if !on.Diverts(c) {
			t.Errorf("%s not diverted", c)
		}
	}
	for _, c := range []domain.ErrorCategory{domain.CategoryStep, domain.CategoryDecode, domain.CategoryOutputInvalid, domain.CategoryDestinationPermanent} {
		if on.After(0, c) != DecisionNone {
			t.Errorf("%s diverted", c)
		}
	}
	cases := map[int]Decision{0: DecisionSchedule, 2: DecisionSchedule, 3: DecisionExhausted, 4: DecisionExhausted}
	for used, want := range cases {
		if got := on.After(used, domain.CategoryDependencyTransient); got != want {
			t.Errorf("after %d: %s want %s", used, got, want)
		}
	}
}

func TestTierNaming(t *testing.T) {
	if got := (TierNaming{}).Topic("Orders Paid", 2); got != "router.retry.orders-paid.2" {
		t.Fatalf("topic %s", got)
	}
	ts := TierNaming{}.Topics("o", 3)
	if len(ts) != 3 || ts[0] != "router.retry.o.1" || ts[2] != "router.retry.o.3" {
		t.Fatalf("topics %v", ts)
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	src := engine.RawRecord{Key: []byte("k"), Value: []byte(`{"a":1}`), Topic: "in", Partition: 3, Offset: 41,
		Timestamp: time.UnixMilli(1_700_000_000_000), Headers: []engine.Header{{Key: "trace", Value: []byte("t1")}, {Key: "router.version", Value: []byte("x")}}}
	env := Envelope{Attempt: 2, DueAt: time.UnixMilli(1_700_000_005_000), ErrorType: record.ErrorLookupUnavailable, Message: "lookup down", Source: src}
	now := time.UnixMilli(1_700_000_001_000)
	raw := EnvelopeCodec{}.Encode(env, "router.retry.o.2", now)
	if raw.Topic != "router.retry.o.2" || !raw.Timestamp.Equal(now) {
		t.Fatalf("raw %+v", raw)
	}
	got, err := EnvelopeCodec{}.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempt != 2 || !got.DueAt.Equal(env.DueAt) || got.ErrorType != env.ErrorType || got.Message != env.Message {
		t.Fatalf("env %+v", got)
	}
	s := got.Source
	if s.Topic != "in" || s.Partition != 3 || s.Offset != 41 || !s.Timestamp.Equal(src.Timestamp) || len(s.Headers) != 1 || s.Headers[0].Key != "trace" {
		t.Fatalf("source %+v", s)
	}
	if _, err := (EnvelopeCodec{}).Decode(engine.RawRecord{Value: []byte("x")}); err != ErrNotTierRecord {
		t.Fatalf("plain record: %v", err)
	}
}

type captureProducer struct{ outs []delivery.Outgoing }

func (p *captureProducer) Produce(_ context.Context, out delivery.Outgoing, onAck delivery.AckFunc) {
	p.outs = append(p.outs, out)
	onAck(nil)
}

func TestSchedulerWritesNextTier(t *testing.T) {
	prod := &captureProducer{}
	now := time.UnixMilli(1_700_000_000_000)
	base := 500
	s := Scheduler{Planner: NewPlanner(&config.ErrorPolicy{Retry: &config.RetryPolicy{Enabled: true, BaseDelayMs: &base}}),
		RouteName: "orders", ConnectionID: "dlq", Producer: prod, Now: func() time.Time { return now }}
	s.Schedule(context.Background(), engine.RawRecord{Value: []byte("v"), Topic: "in"}, 1, Failed{ErrorType: record.ErrorTimeout, Message: "slow"}, func(error) {})
	if len(prod.outs) != 1 || prod.outs[0].ConnectionID != "dlq" || prod.outs[0].Record.Topic != "router.retry.orders.2" {
		t.Fatalf("outs %+v", prod.outs)
	}
	env, err := EnvelopeCodec{}.Decode(prod.outs[0].Record)
	if err != nil || env.Attempt != 2 || !env.DueAt.Equal(now.Add(time.Second)) {
		t.Fatalf("env %+v %v", env, err)
	}
	specs := s.TopicSpecs(0)
	if len(specs) != 5 || specs[0].Partitions != 1 || specs[4].Name != "router.retry.orders.5" || specs[0].RetentionMs != DefaultTierRetention.Millis() {
		t.Fatalf("specs %+v", specs)
	}
}
