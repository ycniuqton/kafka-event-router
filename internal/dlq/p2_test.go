package dlq

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/internal/preview/gateway"
	"github.com/eventrouter/router/pkg/engine"
)

func TestQueryMatchesP2Filters(t *testing.T) {
	r := failure(5, record.ErrorStep)
	r.Failure.Step, r.Failure.VersionNo, r.Failure.Message = "step-1", 3, "Cannot read properties of undefined (reading 'reduce')"
	cases := []struct {
		q    Query
		want bool
	}{
		{Query{Step: "step-1"}, true},
		{Query{Step: "step-2"}, false},
		{Query{VersionNo: 3}, true},
		{Query{VersionNo: 4}, false},
		{Query{Text: "READING 'reduce'"}, true},
		{Query{Text: "timeout"}, false},
		{Query{Step: "step-1", VersionNo: 3, Text: "undefined"}, true},
	}
	for i, c := range cases {
		if c.q.Matches(r) != c.want {
			t.Fatalf("case %d", i)
		}
	}
}

func restoreFixture(t *testing.T) (RestoreAction, ActionContext, []record.Record, *MemoryStateStore) {
	t.Helper()
	state := NewMemoryStateStore()
	loc := Location{Route: RouteInfo{ID: "r1"}, Targets: []Target{{ConnectionID: "c1", Topics: []string{testDLQTopic}}}}
	recs := []record.Record{failure(0, record.ErrorStep), failure(1, record.ErrorStep), failure(2, record.ErrorStep)}
	for i := range recs {
		recs[i].Ref = Ref{Topic: testDLQTopic, Partition: 0, Offset: int64(i)}
	}
	_ = state.Mark(context.Background(), "c1", []Resolution{
		{Ref: recs[0].Ref, RouteID: "r1", State: StateDiscarded, At: baseTime},
		{Ref: recs[1].Ref, RouteID: "r1", State: StateRetried, At: baseTime},
	})
	return RestoreAction{Gate: StateGate{Store: state}}, ActionContext{Location: loc, Now: baseTime}, recs, state
}

func TestRestoreActionOnlyDiscarded(t *testing.T) {
	action, ac, recs, state := restoreFixture(t)
	report := action.Apply(context.Background(), ac, recs)
	byRef := map[Ref]ItemResult{}
	for _, r := range report.Results {
		byRef[r.Ref] = r
	}
	if byRef[recs[0].Ref].Outcome != OutcomeRestored {
		t.Fatalf("discarded not restored: %+v", report.Results)
	}
	for _, r := range recs[1:] {
		if byRef[r.Ref].Reason != SkipNotDiscarded {
			t.Fatalf("%v not skipped: %+v", r.Ref, byRef[r.Ref])
		}
	}
	left, _ := state.Lookup(context.Background(), "c1", "r1", []Ref{recs[0].Ref, recs[1].Ref})
	if _, ok := left[recs[0].Ref]; ok || len(left) != 1 {
		t.Fatalf("state after restore %+v", left)
	}
	if !action.SelectsResolved() || (SelectionOf{}).For(action, Selection{Match: &Query{}}).Match.IncludeResolved != true {
		t.Fatal("restore match must include resolved records")
	}
}

func maskedDetail() DetailView {
	return DetailView{
		Key:     EncodedValue{Encoding: EncodingUTF8, Data: "ord_1"},
		Value:   EncodedValue{Encoding: EncodingJSON, Data: json.RawMessage(`{"email":"a@b.c","amount":12.5,"paid":true,"lines":[{"sku":"X1"}],"note":null}`)},
		Headers: []EncodedHeader{{Key: "traceId", Value: "ab12", Encoding: EncodingUTF8}},
	}
}

func TestMaskingViewPolicyKeepsTypes(t *testing.T) {
	policy := MaskingViewPolicy{Visibility: AlwaysMasked{}, Masker: TypePreservingMasker{}}
	d, err := policy.Present(context.Background(), Viewer{}, "c1", maskedDetail())
	if err != nil || !d.Masked {
		t.Fatalf("%v masked=%v", err, d.Masked)
	}
	got, _ := json.Marshal(d.Value.Data)
	if string(got) != `{"amount":0,"email":"*****","lines":[{"sku":"**"}],"note":null,"paid":false}` {
		t.Fatalf("value %s", got)
	}
	if d.Key.Data != "*****" || d.Headers[0].Value != "****" || d.Headers[0].Key != "traceId" {
		t.Fatalf("key/headers %+v %+v", d.Key, d.Headers)
	}
	rec := failure(0, record.ErrorStep)
	rec.Value, rec.Key = []byte(`{"n":7,"s":"hi"}`), []byte("k1")
	shown, masked, _ := policy.PresentRecord(context.Background(), Viewer{}, "c1", rec)
	if !masked || string(shown.Value) != `{"n":0,"s":"**"}` || string(shown.Key) != "**" {
		t.Fatalf("record %s %s", shown.Value, shown.Key)
	}
}

type rawVisibility struct{}

func (rawVisibility) Masked(context.Context, string) (bool, error) { return false, nil }

func TestMaskingViewPolicyRawForEditors(t *testing.T) {
	policy := MaskingViewPolicy{Visibility: rawVisibility{}, Masker: TypePreservingMasker{}}
	d, _ := policy.Present(context.Background(), Viewer{}, "c1", maskedDetail())
	if d.Masked || d.Key.Data != "ord_1" {
		t.Fatalf("editor view masked: %+v", d)
	}
}

func TestBranchScopeSkipsDeliveredBranches(t *testing.T) {
	outs := []engine.Out{{Branch: "main"}, {Branch: "audit"}, {Branch: "main"}}
	if got := (BranchScope{}).Keep(record.Failure{Branch: "main"}, outs); len(got) != 3 {
		t.Fatalf("no delivered header must keep all branches, got %d", len(got))
	}
	got := (BranchScope{}).Keep(record.Failure{Branch: "main", Delivered: record.BranchSetOf("audit")}, outs)
	if len(got) != 2 || got[0].Branch != "main" || got[1].Branch != "main" {
		t.Fatalf("got %+v", got)
	}
}

func TestNDJSONExporter(t *testing.T) {
	var buf bytes.Buffer
	views := []DetailView{{ItemView: ItemView{ID: "a"}}, {ItemView: ItemView{ID: "b"}}}
	if err := (NDJSONExporter{}).Write(&buf, views); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 || !json.Valid([]byte(lines[1])) || NewExportManager(NDJSONExporter{}).exporters[ExportNDJSON] == nil {
		t.Fatalf("ndjson %q", buf.String())
	}
}

// janitorReader reports low watermark 2 on partition 0 of the DLQ topic.
type janitorReader struct{ Reader }

func (janitorReader) Ranges(context.Context, []Target) (RangeSet, error) {
	return RangeSet{Ranges: []PartitionRange{{Topic: testDLQTopic, Partition: 0, Low: 2, High: 9}}}, nil
}

type staticLocator struct{ loc Location }

func (l staticLocator) Locate(context.Context, string) (Location, error) { return l.loc, nil }

type oneRoute struct{ RouteSource }

func (oneRoute) Routes(context.Context) ([]RouteInfo, error) { return []RouteInfo{{ID: "r1"}}, nil }

func TestStateJanitorTombstonesExpiredKeys(t *testing.T) {
	state := NewMemoryStateStore()
	rs := []Resolution{}
	for o := int64(0); o < 4; o++ {
		rs = append(rs, Resolution{Ref: Ref{Topic: testDLQTopic, Partition: 0, Offset: o}, RouteID: "r1", State: StateDiscarded})
	}
	_ = state.Mark(context.Background(), "c1", rs)
	loc := Location{Route: RouteInfo{ID: "r1"}, Targets: []Target{{ConnectionID: "c1", Topics: []string{testDLQTopic}}}}
	j := StateJanitor{Routes: oneRoute{}, Locator: staticLocator{loc: loc}, Reader: janitorReader{}, State: state,
		Leader: AlwaysLeader{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if n := j.Once(context.Background()); n != 2 {
		t.Fatalf("removed %d", n)
	}
	left, _ := state.Lookup(context.Background(), "c1", "r1", []Ref{rs[0].Ref, rs[1].Ref, rs[2].Ref, rs[3].Ref})
	if len(left) != 2 {
		t.Fatalf("left %+v", left)
	}
}

// fakePreview answers run calls with a canned result.
type fakePreview struct {
	result preview.RunResultDTO
	calls  int
}

func (f *fakePreview) Forward(_ context.Context, _, path string, body []byte, _ preview.Caller) (gateway.Upstream, error) {
	f.calls++
	var req preview.RunRequest
	if err := json.Unmarshal(body, &req); err != nil || !strings.HasSuffix(path, string(preview.EndpointRun)) {
		return gateway.Upstream{}, err
	}
	raw, _ := json.Marshal(f.result)
	return gateway.Upstream{Status: http.StatusOK, Body: raw}, nil
}

func TestPreviewExecutorOutcomes(t *testing.T) {
	out := func(v string) preview.OutputDTO {
		return preview.OutputDTO{Topic: "orders-public", Key: "k", Value: json.RawMessage(v), Valid: true}
	}
	msg := "boom"
	fp := &fakePreview{result: preview.RunResultDTO{Results: []preview.EventResultDTO{
		{Seq: 0, Branches: []preview.BranchResultDTO{{Branch: "main", Outputs: []preview.OutputDTO{out(`{"a":1}`)}}}},
		{Seq: 1, Branches: []preview.BranchResultDTO{{Branch: "main"}}},
		{Seq: 2, Error: &preview.ErrorDTO{ErrorType: string(engine.ErrStep), Message: msg}},
	}}}
	producer := &recordingProducer{}
	target := RetryTarget{Route: RouteInfo{ID: "r1"}, Version: VersionConfig{ID: "v1", No: 1, Config: json.RawMessage(e2eRouteConfig)}}
	recs := []record.Record{failure(0, record.ErrorStep), failure(1, record.ErrorStep), failure(2, record.ErrorStep)}
	results := PreviewExecutor{Preview: fp, Producer: producer}.Execute(context.Background(), target, recs)
	want := []Outcome{OutcomeSent, OutcomeFiltered, OutcomeFailed}
	for i, r := range results {
		if r.Outcome != want[i] {
			t.Fatalf("result %d: %+v", i, r)
		}
	}
	if results[2].ErrorType != FailStep || results[2].Message != msg || len(producer.produced) != 1 || string(producer.produced[0].Value) != `{"a":1}` {
		t.Fatalf("failed/produced %+v %+v", results[2], producer.produced)
	}
}

func TestRemoteRenderingDecodesBase64AndText(t *testing.T) {
	b, err := (RenderedBytes{}).Of(RenderBase64, "3q2+7w==")
	if err != nil || !bytes.Equal(b, []byte{0xde, 0xad, 0xbe, 0xef}) {
		t.Fatalf("%v %v", err, b)
	}
	if b, _ := (RenderedBytes{}).Of(RenderText, "plain"); string(b) != "plain" {
		t.Fatal(string(b))
	}
	if b, _ := (RenderedBytes{}).Of(RenderJSON, json.Number("12345678901234567890")); string(b) != "12345678901234567890" {
		t.Fatal(string(b))
	}
}

// recordingProducer keeps every produced record.
type recordingProducer struct{ produced []engine.RawRecord }

func (p *recordingProducer) Produce(_ context.Context, _ string, recs []engine.RawRecord) ([]Destination, error) {
	p.produced = append(p.produced, recs...)
	return []Destination{{Topic: recs[0].Topic, Offset: int64(len(p.produced))}}, nil
}
