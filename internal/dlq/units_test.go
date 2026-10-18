package dlq

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

func TestRefCodec(t *testing.T) {
	ref := Ref{Topic: "router.dlq.a|b", Partition: 3, Offset: 884162}
	got, err := RefCodec{}.Decode(RefCodec{}.Encode(ref))
	if err != nil || got != ref {
		t.Fatalf("%v %+v", err, got)
	}
	for _, bad := range []string{"", "!!", "eHx5", "fDF8Mg"} {
		if _, err := (RefCodec{}).Decode(bad); !errors.Is(err, ErrInvalidID) {
			t.Fatalf("%q accepted", bad)
		}
	}
	if _, err := (RefCodec{}).DecodeAll([]string{RefCodec{}.Encode(ref), "!!"}); err == nil {
		t.Fatal("bad list accepted")
	}
}

func TestCursorCodec(t *testing.T) {
	c := NewCursor()
	c.Bounds[KeyOf("t", 1)] = 42
	got, err := CursorCodec{}.Decode(CursorCodec{}.Encode(c))
	if err != nil || got.Bounds[KeyOf("t", 1)] != 42 {
		t.Fatalf("%v %+v", err, got)
	}
	old := CursorCodec{}.Encode(Cursor{V: 0, Bounds: map[PartitionKey]int64{}})
	if _, err := (CursorCodec{}).Decode(old); !errors.Is(err, ErrInvalidCursor) {
		t.Fatal("old version accepted")
	}
	if _, err := (CursorCodec{}).DecodeGlobal("bm9wZQ"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatal("junk global accepted")
	}
}

func TestQueryMatches(t *testing.T) {
	r := failure(5, record.ErrorTimeout)
	cases := []struct {
		q    Query
		want bool
	}{
		{Query{}, true},
		{Query{ErrorTypes: []record.ErrorType{record.ErrorStep}}, false},
		{Query{ErrorTypes: []record.ErrorType{record.ErrorStep, record.ErrorTimeout}}, true},
		{Query{Since: baseTime.Add(6 * time.Minute)}, false},
		{Query{Until: baseTime.Add(4 * time.Minute)}, false},
		{Query{Since: baseTime.Add(5 * time.Minute), Until: baseTime.Add(5 * time.Minute)}, true},
	}
	for i, c := range cases {
		if c.q.Matches(r) != c.want {
			t.Fatalf("case %d", i)
		}
	}
	if (Query{Limit: 999}).PageSize() != int(MaxPage) || (Query{}).PageSize() != int(DefaultPage) {
		t.Fatal("page size")
	}
}

func TestPartitionMergerOrderingAndCursor(t *testing.T) {
	s := &ScanState{parts: map[PartitionKey]*PartitionScan{}}
	mk := func(p int32, offsets ...int64) *PartitionScan {
		ps := &PartitionScan{Range: PartitionRange{Topic: "t", Partition: p, Low: 0, High: 100, From: 0, To: 100}, Upper: 100, Cur: 0}
		for _, o := range offsets {
			r := failure(int(o), record.ErrorStep)
			r.Ref = Ref{Topic: "t", Partition: p, Offset: o}
			ps.Matches = append(ps.Matches, r)
		}
		return ps
	}
	s.parts[KeyOf("t", 0)] = mk(0, 9, 5, 1)
	s.parts[KeyOf("t", 1)] = mk(1, 8, 2)
	page := PartitionMerger{}.Page(s, 3)
	if len(page.Records) != 3 || page.Records[0].Ref.Offset != 9 || page.Records[1].Ref.Offset != 8 || page.Records[2].Ref.Offset != 5 {
		t.Fatalf("order %+v", page.Records)
	}
	if page.Next.Done || page.Next.Bounds[KeyOf("t", 0)] != 5 || page.Next.Bounds[KeyOf("t", 1)] != 8 {
		t.Fatalf("cursor %+v", page.Next)
	}
}

func TestMemoryStateStore(t *testing.T) {
	s := NewMemoryStateStore()
	ctx := context.Background()
	ref := Ref{Topic: "t", Partition: 0, Offset: 3}
	_ = s.Mark(ctx, "c", []Resolution{{Ref: ref, RouteID: "r", State: StateRetried}})
	got, _ := s.Lookup(ctx, "c", "r", []Ref{ref})
	other, _ := s.Lookup(ctx, "c2", "r", []Ref{ref})
	if got[ref].State != StateRetried || len(other) != 0 {
		t.Fatal("lookup")
	}
	if n, _ := s.CountResolved(ctx, "c", "r", []PartitionRange{{Topic: "t", Partition: 0, Low: 4, High: 9}}); n != 0 {
		t.Fatal("count below low watermark")
	}
	_ = s.Clear(ctx, "c", "r", []Ref{ref})
	if got, _ := s.Lookup(ctx, "c", "r", []Ref{ref}); len(got) != 0 {
		t.Fatal("clear")
	}
	if route, parsed, ok := (StateKey{}).Parse((StateKey{}).Of("r", ref)); !ok || route != "r" || parsed != ref {
		t.Fatal("state key")
	}
}

type blockingExecutor struct {
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	runs    int
}

func (e *blockingExecutor) Kind() ExecutorKind { return ExecutorInProcess }
func (e *blockingExecutor) Execute(_ context.Context, _ RetryTarget, recs []record.Record) []ItemResult {
	e.mu.Lock()
	e.runs += len(recs)
	e.mu.Unlock()
	if len(recs) > 0 {
		close(e.started)
		<-e.release
	}
	return ResultsOf{}.All(recs, ItemResult{Outcome: OutcomeSent})
}

func TestRetryActionConcurrentSameIDs(t *testing.T) {
	exec := &blockingExecutor{started: make(chan struct{}), release: make(chan struct{})}
	state := NewMemoryStateStore()
	action := RetryAction{Gate: StateGate{Store: state}, Lock: NewMemoryRetryLock(time.Now), Executor: exec}
	loc := Location{Route: RouteInfo{ID: "r1"}, Targets: []Target{{ConnectionID: "c1", Topics: []string{testDLQTopic}}}}
	rec := failure(1, record.ErrorStep)
	rec.Ref = Ref{Topic: testDLQTopic, Offset: 1}
	ac := ActionContext{Location: loc, Target: &RetryTarget{Version: VersionConfig{No: 2}}, Now: baseTime}
	first := make(chan ActionReport)
	go func() { first <- action.Apply(context.Background(), ac, []record.Record{rec}) }()
	<-exec.started
	second := action.Apply(context.Background(), ac, []record.Record{rec})
	close(exec.release)
	r1 := <-first
	if second.Results[0].Reason != SkipLocked || r1.Results[0].Outcome != OutcomeSent || exec.runs != 1 {
		t.Fatalf("first %+v second %+v runs %d", r1.Results, second.Results, exec.runs)
	}
	third := action.Apply(context.Background(), ac, []record.Record{rec})
	if third.Results[0].Reason != SkipAlreadyResolved {
		t.Fatalf("third %+v", third.Results)
	}
}

func TestDiscardActionStateFailure(t *testing.T) {
	loc := Location{Route: RouteInfo{ID: "r1"}, Targets: []Target{{ConnectionID: "c1", Topics: []string{testDLQTopic}}}}
	rec := failure(1, record.ErrorStep)
	rec.Ref = Ref{Topic: testDLQTopic, Offset: 1}
	report := DiscardAction{Gate: StateGate{Store: failingState{}}}.Apply(context.Background(), ActionContext{Location: loc}, []record.Record{rec})
	if report.Results[0].Outcome != OutcomeFailed {
		t.Fatalf("%+v", report)
	}
}

type failingState struct{ *MemoryStateStore }

func (failingState) Lookup(context.Context, string, string, []Ref) (map[Ref]Resolution, error) {
	return map[Ref]Resolution{}, nil
}
func (failingState) Mark(context.Context, string, []Resolution) error { return errors.New("down") }

type fakeProducer struct {
	errs  []error
	calls int
}

func (p *fakeProducer) Produce(_ context.Context, _ string, recs []engine.RawRecord) ([]Destination, error) {
	p.calls++
	if len(p.errs) > 0 {
		err := p.errs[0]
		p.errs = p.errs[1:]
		if err != nil {
			return nil, err
		}
	}
	return []Destination{{Topic: recs[0].Topic, Offset: int64(p.calls)}}, nil
}

func TestInProcessExecutorOutcomes(t *testing.T) {
	route := RouteInfo{ID: "r1", TeamID: "t1"}
	v := VersionConfig{ID: "v1", No: 3, Config: json.RawMessage(e2eRouteConfig)}
	cache := NewPipelineCache(compile.NewDefaultCompiler(), NoScriptRuntimes{}, ctxhelpers.StaticSaltProvider{})
	perm := errors.Join(ErrPermanentDestination, kerr.TopicAuthorizationFailed)
	prod := &fakeProducer{errs: []error{nil, perm}}
	exec := InProcessExecutor{Pipelines: cache, Producer: prod}
	mk := func(i int, v string) record.Record {
		r := failure(i, record.ErrorStep)
		r.Value, r.Ref = []byte(v), Ref{Topic: testDLQTopic, Offset: int64(i)}
		return r
	}
	recs := []record.Record{mk(0, `{"amount":500}`), mk(1, `{"amount":5}`), mk(2, `nope`), mk(3, `{"amount":600}`), mk(4, `{"amount":700}`)}
	got := exec.Execute(context.Background(), RetryTarget{Route: route, Version: v}, recs)
	want := []Outcome{OutcomeSent, OutcomeFiltered, OutcomeFailed, OutcomeFailed, OutcomeFailed}
	for i, w := range want {
		if got[i].Outcome != w {
			t.Fatalf("%d: %+v", i, got[i])
		}
	}
	if got[2].ErrorType != FailDecode || got[3].ErrorType != FailDestination || got[4].ErrorType != FailDestination || prod.calls != 2 {
		t.Fatalf("kinds %+v calls %d", got, prod.calls)
	}
	bad := InProcessExecutor{Pipelines: cache, Producer: prod}.Execute(context.Background(), RetryTarget{Route: route, Version: VersionConfig{ID: "v2", Config: json.RawMessage(`{}`)}}, recs[:1])
	if bad[0].ErrorType != FailCompile {
		t.Fatalf("compile %+v", bad)
	}
}

func TestProduceErrorsClassification(t *testing.T) {
	if !(ProduceErrors{}).Permanent(kerr.TopicAuthorizationFailed) || (ProduceErrors{}).Permanent(kerr.NotLeaderForPartition) || (ProduceErrors{}).Permanent(errors.New("net")) {
		t.Fatal("classification")
	}
}

func TestSettingsLoader(t *testing.T) {
	env := map[string]string{string(EnvDLQStateBackend): "memory"}
	s, err := SettingsLoader{Lookup: func(k string) (string, bool) { v, ok := env[k]; return v, ok }}.Load()
	if err != nil || s.StateBackend != StateBackendMemory || s.Executor != ExecutorInProcess {
		t.Fatalf("%v %+v", err, s)
	}
	env[string(EnvDLQStateBackend)] = "redis"
	if _, err := (SettingsLoader{Lookup: func(k string) (string, bool) { v, ok := env[k]; return v, ok }}).Load(); err == nil {
		t.Fatal("bad backend accepted")
	}
}

func TestStudioLinkAndInline(t *testing.T) {
	l := StudioLinkOf{}.Build("r3", "ses_1")
	if l.BuilderURL != "/routes/r3/edit?sample=0&session=ses_1&step=transform" {
		t.Fatal(l.BuilderURL)
	}
	ev := InlineOf{}.Event(record.Record{Value: []byte("plain"), Key: []byte(`"k"`)})
	if string(ev.Value) != `"plain"` || string(ev.Key) != `"k"` {
		t.Fatalf("%s %s", ev.Value, ev.Key)
	}
}

func FuzzCursorDecode(f *testing.F) {
	f.Add("eyJ2IjoxLCJiIjp7fX0")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = CursorCodec{}.Decode(s)
		_, _ = CursorCodec{}.DecodeGlobal(s)
		_, _ = RefCodec{}.Decode(s)
	})
}
