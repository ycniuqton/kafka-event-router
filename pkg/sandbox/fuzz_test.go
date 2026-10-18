package sandbox_test

import (
	"context"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/boundary"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

// FuzzCheck feeds arbitrary sources to Check: it must return (never panic or hang).
func FuzzCheck(f *testing.F) {
	for _, seed := range []string{"function transform(e){return e}", "}{", "/*", "`${", "function transform(){ for(;;){} }",
		"\x00\xff", "throw 1", "import x from 'y'", "class A { #x }", "async function transform(){}", "'\\u{110000}'"} {
		f.Add(seed, false)
	}
	m := sandboxtest.NewManager(f)
	f.Fuzz(func(t *testing.T, code string, predicate bool) {
		entry := sandbox.EntryTransform
		if predicate {
			entry = sandbox.EntryPredicate
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, d := range m.Check(ctx, sandbox.Source{StepPath: "/f", Entry: entry, Code: code}) {
			if d.Severity != sandbox.SeverityError || d.Message == "" {
				t.Fatalf("bad diagnostic %+v", d)
			}
		}
	})
}

// FuzzInvoke feeds arbitrary event values through a fixed transform: results are either
// valid outputs or typed errors.
func FuzzInvoke(f *testing.F) {
	for _, seed := range []string{"", "x", "\xff\xfe", "{\"a\":1}", " ", "null"} {
		f.Add(seed, int64(1), "k")
	}
	m := sandboxtest.NewManager(f)
	mod, _ := sandboxtest.LoadOne(f, m, sandbox.EntryTransform, `function transform(event, ctx) {
  const v = event.value;
  if (typeof v === 'string' && v.length % 3 === 0) return null;
  if (typeof v === 'string' && v.length % 3 === 1) return [{ value: v }, { value: ctx.hash(v), headers: event.headers }];
  return { key: event.key, value: { v, masked: ctx.mask.last4(v), offset: event.offset } };
}`)
	f.Fuzz(func(t *testing.T, value string, offset int64, header string) {
		ev := boundary.GuestEvent{Value: value, Key: header, Offset: offset, Headers: []boundary.Header{{Key: header, Value: []byte(value)}}}
		res, err := mod.Call(context.Background(), sandbox.Invocation{Event: ev, Now: time.Unix(1, 0)})
		if err != nil {
			if _, ok := sandbox.AsError(err); !ok {
				t.Fatalf("untyped error %v", err)
			}
			return
		}
		if !res.Dropped && len(res.Outputs) == 0 {
			t.Fatalf("neither dropped nor outputs: %+v", res)
		}
		if utf8.ValidString(value) && !res.Dropped && len(res.Outputs) == 1 {
			got := res.Outputs[0].Value.(map[string]any)["v"]
			if got != value {
				t.Fatalf("value %q round-tripped as %q", value, got)
			}
		}
	})
}
