package compile_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	jsonpatch "github.com/evanphx/json-patch/v5"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/enginetest"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/engine/scripttest"
	"github.com/eventrouter/router/pkg/engine/trace"
)

var ctx = context.Background()

func TestTraceFilteredRecordsFailingNode(t *testing.T) {
	p := enginetest.MustCompile(t, enginetest.RouteJSON(`{"type":"and","children":[
		{"type":"cond","field":"value.a","op":"eq","value":1},{"type":"cond","field":"value.b","op":"gt","value":10}]}`, `[]`))
	tr := trace.NewRecordingTracer(5)
	outs, err := p.Process(ctx, enginetest.MustDecode(t, p, `{"a":1,"b":3}`), tr)
	if err != nil || len(outs) != 0 {
		t.Fatal(outs, err)
	}
	got := tr.Trace()
	f := got.Branches[0].Filter
	if got.Status != trace.StatusFiltered || got.Offset != 5 || f[1].Node != "/children/1" || f[1].Result {
		t.Fatalf("trace %+v", got)
	}
}

func TestTracePatchesReproduceEachStep(t *testing.T) {
	steps := `[{"id":"r","kind":"rename","from":"value.cust_id","to":"value.customerId"},
	 {"id":"m","kind":"mask","field":"value.email","style":"email"},
	 {"id":"s","kind":"set","field":"headers.x","source":{"from":"constant","value":"y"}},
	 {"id":"k","kind":"set_key","source":{"from":"field","path":"value.customerId"}},
	 {"id":"d","kind":"remove","fields":["value.tmp"]},
	 {"id":"split","kind":"js","code":"golden:split-items"}]`
	p := enginetest.MustCompile(t, enginetest.RouteJSON("null", steps))
	ev := enginetest.MustDecode(t, p, `{"cust_id":"C1","email":"jo@x.io","tmp":[1,{"a":null}],"orderId":"o","items":[{"sku":"A"},{"sku":"B"}]}`)
	before := []any{ev.Clone().Snapshot()}
	tr := trace.NewRecordingTracer(0)
	outs, err := p.Process(ctx, ev, tr)
	if err != nil || len(outs) != 2 {
		t.Fatal(outs, err)
	}
	doc := mustJSON(t, before[0])
	for i, st := range tr.Trace().Branches[0].Steps {
		patch := mustJSON(t, st.Patch)
		if i == len(tr.Trace().Branches[0].Steps)-1 {
			doc = mustJSON(t, []any{json.RawMessage(doc)})
		}
		decoded, err := jsonpatch.DecodePatch(patch)
		if err != nil {
			t.Fatal(err)
		}
		doc, err = decoded.Apply(doc)
		if err != nil {
			t.Fatalf("step %s: %v (patch %s)", st.ID, err, patch)
		}
	}
	want := []any{}
	for _, o := range outs {
		want = append(want, o.Event.Snapshot())
	}
	if !jsonpatch.Equal(doc, mustJSON(t, want)) {
		t.Fatalf("patched %s\nwant %s", doc, mustJSON(t, want))
	}
	if logs := tr.Trace().Branches[0].Steps[5].Logs; len(logs) != 1 || logs[0] != "split" {
		t.Fatalf("logs %v", logs)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestProcessErrorsAreTyped(t *testing.T) {
	rt := scripttest.NewFuncRuntime().
		Register("slow", func(context.Context, script.Invocation) (script.Result, error) {
			return script.Result{}, script.ErrTimeout
		}).
		Register("boom", func(context.Context, script.Invocation) (script.Result, error) {
			return script.Result{}, errors.New("TypeError")
		}).
		Register("bad", func(context.Context, script.Invocation) (script.Result, error) {
			return script.Result{Value: "str"}, nil
		}).
		Register("nil", func(context.Context, script.Invocation) (script.Result, error) { return script.Result{}, nil })
	cases := []struct {
		filter, steps string
		want          engine.ErrorType
		path          config.Pointer
		status        trace.EventStatus
	}{
		{`{"type":"expr","code":"slow"}`, `[]`, engine.ErrStepTimeout, "/branches/0/filter", trace.StatusError},
		{`{"type":"not","child":{"type":"expr","code":"boom"}}`, `[]`, engine.ErrStep, "/branches/0/filter/child", trace.StatusError},
		{"null", `[{"kind":"js","code":"slow"}]`, engine.ErrStepTimeout, "/branches/0/transform/steps/0", trace.StatusError},
		{"null", `[{"kind":"js","code":"boom"}]`, engine.ErrStep, "/branches/0/transform/steps/0", trace.StatusError},
		{"null", `[{"kind":"js","code":"bad"}]`, engine.ErrStep, "/branches/0/transform/steps/0", trace.StatusError},
		{"null", `[{"kind":"js","code":"nil"}]`, "", "", trace.StatusDropped},
	}
	for _, c := range cases {
		doc, _ := config.Parse([]byte(enginetest.RouteJSON(c.filter, c.steps)))
		opts := enginetest.GoldenOptions()
		opts.Scripts = rt
		p, err := compile.NewDefaultCompiler().Compile(ctx, doc, opts)
		if err != nil {
			t.Fatal(err)
		}
		tr := trace.NewRecordingTracer(0)
		_, err = p.Process(ctx, enginetest.MustDecode(t, p, `{}`), tr)
		var pe *engine.ProcessError
		if c.want == "" {
			if err != nil || tr.Trace().Status != c.status {
				t.Fatalf("%s: %v %s", c.steps, err, tr.Trace().Status)
			}
			continue
		}
		if !errors.As(err, &pe) || pe.Type != c.want || pe.Path != c.path || tr.Trace().Status != c.status || tr.Trace().Error.Type != string(c.want) {
			t.Fatalf("%s %s: got %+v trace %+v", c.filter, c.steps, err, tr.Trace())
		}
		_ = p.Close(ctx)
	}
}

func TestDecodeEncodeAndLimits(t *testing.T) {
	route := `{"apiVersion":"router/v1","name":"x","source":{"connectionId":"c","format":{"type":"json"},"keyFormat":{"type":"json"}},
	 "branches":[{"id":"main","destination":{"connectionId":"c","topic":"out","key":{"mode":"none"},"headers":{"mode":"drop_all","add":[{"key":"a","value":"b"}]},"format":{"type":"string"}}}],
	 "limits":{"maxOutputBytes":10}}`
	p := enginetest.MustCompile(t, route)
	if _, err := p.Decode(ctx, engine.RawRecord{Key: []byte("{bad"), Value: []byte(`{}`)}); !isType(err, engine.ErrDecode) || !strings.HasSuffix(string(asPE(err).Path), "/keyFormat") {
		t.Fatalf("key decode: %v", err)
	}
	ev, err := p.Decode(ctx, engine.RawRecord{Key: []byte(`{"id":1}`), Value: []byte(`"hello"`), Headers: []engine.Header{{Key: "h", Value: []byte("v")}}})
	if err != nil {
		t.Fatal(err)
	}
	outs, _ := p.Process(ctx, ev, nil)
	rec, err := p.Encode(ctx, outs[0])
	if err != nil || rec.Key != nil || string(rec.Value) != "hello" || rec.Topic != "out" || len(rec.Headers) != 1 || rec.Headers[0].Key != "a" {
		t.Fatalf("encode %+v %v", rec, err)
	}
	outs[0].Value = "this is much longer than ten bytes"
	if _, err := p.Encode(ctx, outs[0]); !isType(err, engine.ErrOutputInvalid) || !errors.Is(err, compile.ErrOutputTooLarge) {
		t.Fatalf("size limit: %v", err)
	}
	outs[0].Value = map[string]any{"not": "a string"}
	if _, err := p.Encode(ctx, outs[0]); !isType(err, engine.ErrOutputInvalid) {
		t.Fatalf("string codec on map: %v", err)
	}
	if _, err := p.Encode(ctx, engine.Out{Branch: "nope"}); !isType(err, engine.ErrOutputInvalid) {
		t.Fatalf("unknown branch: %v", err)
	}
}

func TestCompileErrorsCollectAllIssues(t *testing.T) {
	route := `{"apiVersion":"router/v1","name":"x","source":{"connectionId":"c","format":{"type":"xml"},"keyFormat":{"type":"yaml"}},
	 "branches":[{"id":"main","filter":{"type":"cond","field":"bad..","op":"eq","value":1},
	  "transform":{"steps":[{"kind":"rename","from":"value.a","to":"topic"},{"kind":"js","code":"x"}]},
	  "destination":{"connectionId":"c","topic":"out","key":{"mode":"field","field":"nope.x"}}}]}`
	doc, _ := config.Parse([]byte(route))
	_, err := compile.NewDefaultCompiler().Compile(ctx, doc, engine.CompileOptions{})
	var ce *engine.CompileError
	if !errors.As(err, &ce) {
		t.Fatal(err)
	}
	paths := map[config.Pointer]config.IssueCode{}
	for _, i := range ce.Issues {
		paths[i.Path] = i.Code
	}
	want := map[config.Pointer]config.IssueCode{
		"/source/format/type":                config.IssueFormatUnsupported,
		"/source/keyFormat/type":             config.IssueFormatUnsupported,
		"/branches/0/filter/field":           config.IssuePathInvalid,
		"/branches/0/transform/steps/1/code": config.IssueJSCompile,
		"/branches/0/destination/key/field":  config.IssuePathInvalid,
	}
	for p, c := range want {
		if paths[p] != c {
			t.Errorf("missing %s %s in %+v", p, c, ce.Issues)
		}
	}
	bad, _ := config.Parse([]byte(strings.Replace(route, "router/v1", "router/v2", 1)))
	if _, err := compile.NewDefaultCompiler().Compile(ctx, bad, engine.CompileOptions{}); !errors.As(err, &ce) || ce.Issues[0].Code != config.IssueVersionUnsupported {
		t.Fatalf("version: %v", err)
	}
}

func TestRenameToReadOnlyIsStepError(t *testing.T) {
	p := enginetest.MustCompile(t, enginetest.RouteJSON("null", `[{"kind":"rename","from":"value.a","to":"topic"}]`))
	_, err := p.Process(ctx, enginetest.MustDecode(t, p, `{"a":1}`), nil)
	if !isType(err, engine.ErrStep) {
		t.Fatalf("%v", err)
	}
}

func isType(err error, t engine.ErrorType) bool {
	pe := asPE(err)
	return pe != nil && pe.Type == t
}

func asPE(err error) *engine.ProcessError {
	var pe *engine.ProcessError
	if errors.As(err, &pe) {
		return pe
	}
	return nil
}
