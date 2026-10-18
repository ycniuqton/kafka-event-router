package filter_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compilectx"
	"github.com/eventrouter/router/pkg/engine/filter"
	"github.com/eventrouter/router/pkg/engine/filter/operators"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/engine/scripttest"
	"github.com/eventrouter/router/pkg/engine/trace"
)

var now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func compileNode(t testing.TB, raw string, rt script.Runtime) (filter.Node, config.Issues) {
	t.Helper()
	var spec config.FilterNode
	if err := (config.JSONDecoder{}).Into([]byte(raw), &spec); err != nil {
		t.Fatal(err)
	}
	scope := &compilectx.Scope{Ctx: context.Background(), Modules: &script.ModuleSet{}, Scripts: rt}
	nc := &filter.NodeCompileCtx{Scope: scope, Base: "/f", Nodes: filter.DefaultNodeCompilerManager(), Operators: operators.Default()}
	n, _ := nc.Compile(config.PointerRoot, spec)
	return n, scope.Issues
}

func eval(t *testing.T, n filter.Node, value any, tr trace.Tracer) bool {
	t.Helper()
	ev := &engine.Event{Value: value}
	ok, err := n.Eval(&filter.EvalContext{Ctx: context.Background(), Event: ev, Now: now, Branch: "main", Tracer: trace.OrNoop(tr)})
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

type opCase struct {
	node    string
	left    any
	want    bool
	warning filter.WarningCode
}

func TestOperatorsByCoercionRow(t *testing.T) {
	cond := func(op, operand string) string {
		return `{"type":"cond","field":"value.x","op":"` + op + `"` + operand + `}`
	}
	cases := []opCase{
		{cond("eq", `,"value":100`), int64(100), true, ""},
		{cond("eq", `,"value":"100"`), int64(100), true, ""},
		{cond("eq", `,"value":100`), "100", true, filter.WarnStringToNumber},
		{cond("eq", `,"value":true`), "true", true, filter.WarnStringToBool},
		{cond("eq", `,"value":"true"`), true, true, filter.WarnStringToBool},
		{cond("eq", `,"value":"a"`), "a", true, ""},
		{cond("eq", `,"value":"a"`), []any{"a"}, false, filter.WarnIncomparable},
		{cond("eq", `,"value":null`), nil, true, ""},
		{cond("eq", `,"value":9007199254740993`), int64(9007199254740993), true, ""},
		{cond("eq", `,"value":9007199254740993`), int64(9007199254740992), false, ""},
		{cond("ne", `,"value":1`), int64(2), true, ""},
		{cond("ne", `,"value":1`), "1", false, filter.WarnStringToNumber},
		{cond("gt", `,"value":100`), 100.5, true, ""},
		{cond("gt", `,"value":100`), "150", true, filter.WarnStringToNumber},
		{cond("gt", `,"value":100`), "abc", false, filter.WarnIncomparable},
		{cond("gt", `,"value":"b"`), "c", true, ""},
		{cond("gte", `,"value":100`), int64(100), true, ""},
		{cond("lt", `,"value":100`), int64(99), true, ""},
		{cond("lte", `,"value":100`), int64(101), false, ""},
		{cond("between", `,"from":1,"to":10`), int64(10), true, ""},
		{cond("between", `,"from":1,"to":10`), int64(11), false, ""},
		{cond("between", `,"from":1,"to":10`), "x", false, filter.WarnIncomparable},
		{cond("in", `,"values":["a",1]`), int64(1), true, ""},
		{cond("in", `,"values":["a",1]`), "b", false, ""},
		{cond("not_in", `,"values":["a",1]`), "b", true, ""},
		{cond("not_in", `,"values":["a",1]`), "a", false, ""},
		{cond("contains", `,"value":"ell"`), "hello", true, ""},
		{cond("contains", `,"value":"vip"`), []any{"eu", "vip"}, true, ""},
		{cond("contains", `,"value":"x"`), int64(5), false, filter.WarnIncomparable},
		{cond("starts_with", `,"value":"he"`), "hello", true, ""},
		{cond("ends_with", `,"value":"lo"`), "hello", true, ""},
		{cond("ends_with", `,"value":"lo"`), int64(1), false, filter.WarnIncomparable},
		{cond("regex", `,"value":"^a\\d+$"`), "a12", true, ""},
		{cond("regex", `,"value":"^a\\d+$"`), "a12b", false, ""},
		{cond("exists", ``), nil, true, ""},
		{cond("is_empty", ``), nil, true, ""},
		{cond("is_empty", ``), "", true, ""},
		{cond("is_empty", ``), []any{}, true, ""},
		{cond("is_empty", ``), map[string]any{}, true, ""},
		{cond("is_empty", ``), "x", false, ""},
		{cond("before", `,"value":"2026-01-01T00:00:00Z"`), "2025-12-31T23:59:59Z", true, ""},
		{cond("before", `,"value":1767225600000`), float64(1767225599000), true, ""},
		{cond("after", `,"value":"2025-01-01T00:00:00Z"`), "2025-06-01T00:00:00Z", true, ""},
		{cond("after", `,"value":"2025-01-01T00:00:00Z"`), "yesterday", false, filter.WarnNotADate},
		{cond("within_last", `,"amount":1,"unit":"hours"`), "2025-12-31T23:30:00Z", true, ""},
		{cond("within_last", `,"amount":1,"unit":"hours"`), "2025-12-31T22:30:00Z", false, ""},
		{cond("within_last", `,"amount":1,"unit":"hours"`), "2026-01-01T00:00:01Z", false, ""},
		{cond("within_last", `,"amount":0.5,"unit":"days"`), "2025-12-31T13:00:00Z", true, ""},
	}
	for _, c := range cases {
		n, issues := compileNode(t, c.node, nil)
		if len(issues) > 0 {
			t.Fatalf("%s: %+v", c.node, issues)
		}
		rt := trace.NewRecordingTracer(0)
		got := eval(t, n, map[string]any{"x": c.left}, rt)
		res := rt.Trace().Branches[0].Filter[0]
		if got != c.want || res.Warning != string(c.warning) {
			t.Errorf("%s left=%#v: got %v warning %q, want %v %q", c.node, c.left, got, res.Warning, c.want, c.warning)
		}
	}
}

func TestMissingFieldRule(t *testing.T) {
	for _, op := range []string{`"op":"eq","value":1`, `"op":"ne","value":1`, `"op":"not_in","values":[1]`, `"op":"lt","value":1`} {
		n, _ := compileNode(t, `{"type":"cond","field":"value.missing",`+op+`}`, nil)
		if eval(t, n, map[string]any{}, nil) {
			t.Errorf("%s on missing must be false", op)
		}
	}
	for op, want := range map[string]bool{"exists": false, "not_exists": true, "is_empty": true} {
		n, _ := compileNode(t, `{"type":"cond","field":"value.missing","op":"`+op+`"}`, nil)
		if eval(t, n, map[string]any{}, nil) != want {
			t.Errorf("%s on missing want %v", op, want)
		}
	}
}

func TestCompileIssuesCarryPointers(t *testing.T) {
	cases := map[string]config.Pointer{
		`{"type":"cond","field":"value..a","op":"eq","value":1}`:                  "/f/field",
		`{"type":"cond","field":"value.a","op":"regex","value":"("}`:              "/f/value",
		`{"type":"cond","field":"value.a","op":"regex","value":5}`:                "/f/value",
		`{"type":"cond","field":"value.a","op":"before","value":"soon"}`:          "/f/value",
		`{"type":"cond","field":"value.a","op":"within_last","amount":1}`:         "/f/amount",
		`{"type":"cond","field":"value.a","op":"bogus","value":1}`:                "/f/op",
		`{"type":"and","children":[{"type":"cond","field":"x.a","op":"exists"}]}`: "/f/children/0/field",
		`{"type":"not","child":{"type":"expr","code":"x"}}`:                       "/f/child/code",
		`{"type":"nand"}`: "/f/type",
		`{"type":"cond","field":"value.a","op":"starts_with","value":1}`: "/f/value",
	}
	for raw, want := range cases {
		_, issues := compileNode(t, raw, nil)
		if len(issues) != 1 || issues[0].Path != want {
			t.Errorf("%s: want issue at %s, got %+v", raw, want, issues)
		}
	}
}

func TestShortCircuitAndTrace(t *testing.T) {
	n, _ := compileNode(t, `{"type":"and","children":[
		{"type":"cond","field":"value.a","op":"eq","value":1},
		{"type":"or","children":[{"type":"cond","field":"value.b","op":"eq","value":2},{"type":"cond","field":"value.c","op":"eq","value":3}]},
		{"type":"not","child":{"type":"cond","field":"value.d","op":"exists"}}]}`, nil)
	rt := trace.NewRecordingTracer(7)
	if eval(t, n, map[string]any{"a": int64(2)}, rt) {
		t.Fatal("want false")
	}
	nodes := rt.Trace().Branches[0].Filter
	if len(nodes) != 2 || nodes[0].Node != "/children/0" || nodes[0].Result || nodes[1].Node != "" {
		t.Fatalf("and must stop at first false: %+v", nodes)
	}
	rt = trace.NewRecordingTracer(7)
	if !eval(t, n, map[string]any{"a": int64(1), "b": int64(2)}, rt) {
		t.Fatal("want true")
	}
	var ids []string
	for _, r := range rt.Trace().Branches[0].Filter {
		ids = append(ids, r.Node)
	}
	want := []string{"/children/0", "/children/1/children/0", "/children/1", "/children/2/child", "/children/2", ""}
	if len(ids) != len(want) {
		t.Fatalf("or must stop at first true: %v", ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("trace order %v", ids)
		}
	}
}

func TestExprNode(t *testing.T) {
	rt := scripttest.NewFuncRuntime().
		Register("bool", func(_ context.Context, inv script.Invocation) (script.Result, error) {
			return script.Result{Value: inv.Event["value"].(map[string]any)["ok"]}, nil
		}).
		Register("timeout", func(context.Context, script.Invocation) (script.Result, error) {
			return script.Result{}, script.ErrTimeout
		})
	n, issues := compileNode(t, `{"type":"expr","code":"bool"}`, rt)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	if !eval(t, n, map[string]any{"ok": true}, nil) || eval(t, n, map[string]any{"ok": false}, nil) {
		t.Fatal("bool predicate")
	}
	tr := trace.NewRecordingTracer(0)
	if !eval(t, n, map[string]any{"ok": "yes"}, tr) || tr.Trace().Branches[0].Filter[0].Warning != string(filter.WarnNonBooleanExpr) {
		t.Fatal("truthiness warning")
	}
	n, _ = compileNode(t, `{"type":"expr","code":"timeout"}`, rt)
	_, err := n.Eval(&filter.EvalContext{Ctx: context.Background(), Event: &engine.Event{}, Tracer: trace.NoopTracer{}})
	var fe *filter.FilterError
	if err == nil || !errorsAs(err, &fe) || fe.Pointer != "" {
		t.Fatalf("want FilterError, got %v", err)
	}
	if _, issues := compileNode(t, `{"type":"expr","code":"x"}`, nil); len(issues) != 1 || issues[0].Code != config.IssueJSCompile {
		t.Fatalf("no runtime: %+v", issues)
	}
	for v, want := range map[any]bool{nil: false, "": false, int64(0): false, 1.5: true, "a": true} {
		if (filter.Truthiness{}).Of(v) != want {
			t.Errorf("truthiness %v", v)
		}
	}
}

func FuzzFilterASTRoundTrip(f *testing.F) {
	seeds := []string{
		`{"type":"and","children":[{"type":"cond","field":"value.a","op":"eq","value":1},{"type":"not","child":{"type":"cond","field":"value.b","op":"in","values":["x",2.5,null]}}]}`,
		`{"type":"or","children":[{"type":"cond","field":"value.t","op":"within_last","amount":3,"unit":"days"},{"type":"cond","field":"headers.h","op":"between","from":1,"to":"z"}]}`,
		`{"type":"cond","field":"value.n","op":"gt","value":12345678901234567890}`,
		`{"type":"cond","field":"value.s","op":"regex","value":"^a+$"}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		var spec config.FilterNode
		if err := (config.JSONDecoder{}).Into(raw, &spec); err != nil {
			return
		}
		canonIn, err := json.Marshal(spec)
		if err != nil {
			return
		}
		n, issues := compileNode(t, string(canonIn), nil)
		if len(issues) > 0 || n == nil {
			return
		}
		out, err := json.Marshal(n.Spec())
		if err != nil || string(out) != string(canonIn) {
			t.Fatalf("round trip changed AST:\n in  %s\n out %s", canonIn, out)
		}
		eval(t, n, map[string]any{"a": int64(1), "s": "aa"}, trace.NewRecordingTracer(0))
	})
}
