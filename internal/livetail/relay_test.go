package livetail_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/livetail"
	"github.com/eventrouter/router/internal/rbac"
)

func sampleRecord(outcome livetail.TailOutcome, email string) livetail.TailRecord {
	in := livetail.TailEvent{Topic: "orders", Offset: 7, Encoding: livetail.EncodingJSON,
		Value: map[string]any{"email": email, "amount": 12.5, "vip": true}, Key: "k-1",
		Headers: []livetail.Header{{Key: "trace", Value: "abc"}}}
	out := livetail.TailEvent{Topic: "out", Encoding: livetail.EncodingText, Value: "plain " + email}
	return livetail.TailRecord{At: time.Unix(10, 0).UTC(), Offset: 7, Outcome: outcome, Input: in, Outputs: []livetail.TailEvent{out}}
}

func TestTypePreservingTailMaskerKeepsTypes(t *testing.T) {
	rec := livetail.NewTypePreservingTailMasker().Mask(sampleRecord(livetail.OutcomeSent, "ann@example.com"))
	v := rec.Input.Value.(map[string]any)
	if v["email"] == "ann@example.com" || v["amount"] != 0 || v["vip"] != false {
		t.Fatalf("input not masked by type: %v", v)
	}
	if _, ok := v["email"].(string); !ok {
		t.Fatal("masked string lost its type")
	}
	if rec.Input.Key == "k-1" || rec.Input.Headers[0].Value == "abc" {
		t.Fatal("key/header not masked")
	}
	if rec.Outputs[0].Value == "plain ann@example.com" || rec.Input.Topic != "orders" || rec.Input.Offset != 7 {
		t.Fatalf("output text not masked or metadata changed: %+v", rec)
	}
}

// fakeSamples answers SampleAccess with a fixed decision.
type fakeSamples struct {
	masked bool
	err    error
}

func (f fakeSamples) Decide(context.Context, rbac.SampleSurface, string) (rbac.SampleDecision, error) {
	return rbac.SampleDecision{Masked: f.masked, Action: audit.ActionLiveTailOpened}, f.err
}

func TestRBACVisibilityRawAndMaskedAndFailClosed(t *testing.T) {
	ctx := context.Background()
	if (livetail.RBACVisibility{Access: fakeSamples{}}).Masked(ctx, "c1") {
		t.Fatal("raw permission masked")
	}
	if !(livetail.RBACVisibility{Access: fakeSamples{masked: true}}).Masked(ctx, "c1") {
		t.Fatal("viewer not masked")
	}
	if !(livetail.RBACVisibility{Access: fakeSamples{err: errors.New("down")}}).Masked(ctx, "c1") {
		t.Fatal("error did not fail closed")
	}
}

func TestTailFilter(t *testing.T) {
	rec := sampleRecord(livetail.OutcomeFiltered, "bob@example.com")
	raw, _ := json.Marshal(rec)
	cases := []struct {
		op   livetail.ClientOp
		want bool
	}{
		{livetail.ClientOp{}, true},
		{livetail.ClientOp{Outcomes: []livetail.TailOutcome{livetail.OutcomeError}}, false},
		{livetail.ClientOp{Outcomes: []livetail.TailOutcome{livetail.OutcomeFiltered, "bogus"}}, true},
		{livetail.ClientOp{Text: "BOB@"}, true},
		{livetail.ClientOp{Text: "carol"}, false},
	}
	for i, c := range cases {
		if got := livetail.FilterOf(c.op).Match(rec, raw); got != c.want {
			t.Errorf("case %d: got %v", i, got)
		}
	}
}

// memStore is an in-memory TailStore fed by the test.
type memStore struct {
	mu      sync.Mutex
	watches int
	feed    chan livetail.StreamEntry
	fail    error
}

func (s *memStore) Watch(context.Context, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watches++
	return s.fail
}

func (s *memStore) Read(ctx context.Context, _ string, _ string) ([]livetail.StreamEntry, error) {
	select {
	case e := <-s.feed:
		return []livetail.StreamEntry{e}, nil
	case <-ctx.Done():
		return nil, nil
	case <-time.After(20 * time.Millisecond):
		return nil, nil
	}
}

// pipeConn is a FrameConn whose ops come from the test and frames go to a channel.
type pipeConn struct {
	ops    chan livetail.ClientOp
	frames chan livetail.Frame
}

func (c *pipeConn) WriteJSON(_ context.Context, v any) error {
	c.frames <- v.(livetail.Frame)
	return nil
}

func (c *pipeConn) ReadJSON(ctx context.Context, v any) error {
	select {
	case op := <-c.ops:
		*(v.(*livetail.ClientOp)) = op
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func entry(t *testing.T, rec livetail.TailRecord) livetail.StreamEntry {
	t.Helper()
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	return livetail.StreamEntry{ID: "1-0", Kind: livetail.EntryRecord, Data: data}
}

func nextFrame(t *testing.T, c *pipeConn) livetail.Frame {
	t.Helper()
	select {
	case f := <-c.frames:
		return f
	case <-time.After(2 * time.Second):
		t.Fatal("no frame")
	}
	return livetail.Frame{}
}

func TestTailSessionFilterPauseAndMasking(t *testing.T) {
	store := &memStore{feed: make(chan livetail.StreamEntry)}
	conn := &pipeConn{ops: make(chan livetail.ClientOp), frames: make(chan livetail.Frame, 16)}
	s := &livetail.TailSession{RouteID: "r1", Store: store, Masker: livetail.NewTypePreservingTailMasker(), Masked: true,
		Limits: livetail.DefaultTailLimits(), Log: quiet}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx, conn) }()
	if f := nextFrame(t, conn); f.Type != livetail.FrameStatus || f.State != livetail.StateLive {
		t.Fatalf("first frame %+v", f)
	}
	store.feed <- entry(t, sampleRecord(livetail.OutcomeSent, "ann@example.com"))
	f := nextFrame(t, conn)
	if f.Type != livetail.FrameRecord || !f.Masked || f.Record.Input.Value.(map[string]any)["email"] == "ann@example.com" {
		t.Fatalf("record frame not masked: %+v", f)
	}
	conn.ops <- livetail.ClientOp{Op: livetail.OpFilter, Outcomes: []livetail.TailOutcome{livetail.OutcomeError}}
	time.Sleep(100 * time.Millisecond)
	store.feed <- entry(t, sampleRecord(livetail.OutcomeSent, "x"))
	store.feed <- entry(t, sampleRecord(livetail.OutcomeError, "y"))
	if f := nextFrame(t, conn); f.Record == nil || f.Record.Outcome != livetail.OutcomeError {
		t.Fatalf("filter let through %+v", f)
	}
	conn.ops <- livetail.ClientOp{Op: livetail.OpPause}
	if f := nextFrame(t, conn); f.State != livetail.StatePaused {
		t.Fatalf("pause ack %+v", f)
	}
	store.feed <- entry(t, sampleRecord(livetail.OutcomeError, "z"))
	time.Sleep(100 * time.Millisecond)
	conn.ops <- livetail.ClientOp{Op: livetail.OpResume}
	if f := nextFrame(t, conn); f.State != livetail.StateLive {
		t.Fatalf("paused record delivered or no resume ack: %+v", f)
	}
	store.feed <- livetail.StreamEntry{ID: "2-0", Kind: livetail.EntryThrottled}
	if f := nextFrame(t, conn); f.State != livetail.StateThrottled {
		t.Fatalf("throttle frame %+v", f)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestTailSessionUnavailableWhenWatchFails(t *testing.T) {
	store := &memStore{feed: make(chan livetail.StreamEntry), fail: errors.New("redis down")}
	conn := &pipeConn{ops: make(chan livetail.ClientOp), frames: make(chan livetail.Frame, 1)}
	s := &livetail.TailSession{RouteID: "r1", Store: store, Limits: livetail.DefaultTailLimits(), Log: quiet}
	if err := s.Run(context.Background(), conn); !errors.Is(err, livetail.ErrTailUnavailable) {
		t.Fatalf("err = %v", err)
	}
}
