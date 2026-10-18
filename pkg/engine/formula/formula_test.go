package formula_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/formula"
)

type lookupTables map[string]map[string]any

func (l lookupTables) Lookup(table, key string) (any, bool) {
	v, ok := l[table][key]
	return v, ok
}

var now = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func evalCtx() *formula.EvalContext {
	h := ctxhelpers.DefaultSet()
	h.Lookup = lookupTables{"countries": {"DE": "Germany"}}
	ev := &engine.Event{
		Value: map[string]any{
			"first": "Ada", "last": "Lovelace", "price": 10.0, "qty": int64(3), "s": "  Mixed Case ",
			"items": []any{int64(1), int64(2)}, "n": nil, "num": "4.5", "cc": "DE", "ts": "2026-03-04T10:00:00Z",
			"nested": map[string]any{"a": []any{map[string]any{"b": "deep"}}},
		},
		Headers: []engine.Header{{Key: "retries", Value: []byte("2")}},
		Topic:   "orders",
	}
	return &formula.EvalContext{Ctx: context.Background(), Event: ev, Now: now, Helpers: h}
}

func TestEval(t *testing.T) {
	cases := map[string]any{
		`concat(value.first, " ", value.last)`: "Ada Lovelace",
		`concat("x", value.n, 1, true)`:        "x1true",
		`upper(value.first)`:                   "ADA",
		`lower("ABC")`:                         "abc",
		`trim(value.s)`:                        "Mixed Case",
		`substring("abcdef", 1, 3)`:            "bcd",
		`substring("abc", 5)`:                  "",
		`substring("äöü", 1)`:                  "öü",
		`len(value.items)`:                     int64(2),
		`len("äb")`:                            int64(2),
		`len(value.missing)`:                   int64(0),
		`round(value.price * 1.1, 2)`:          11.0,
		`round(2.5)`:                           int64(3),
		`round(-2.5)`:                          int64(-3),
		`value.qty * 2 + 1`:                    int64(7),
		`value.qty / 2`:                        1.5,
		`7 % 4`:                                int64(3),
		`-value.qty`:                           int64(-3),
		`(1 + 2) * 3`:                          int64(9),
		`1 + 2 * 3`:                            int64(7),
		`value.num + 1`:                        5.5,
		`coalesce(value.n, value.missing, "fallback")`: "fallback",
		`if(value.qty > 2, "many", "few")`:             "many",
		`if(value.qty > 5, "many")`:                    nil,
		`if(true, "ok", toNumber("x"))`:                "ok",
		`toNumber(headers.retries)`:                    int64(2),
		`toNumber("1.25")`:                             1.25,
		`toString(value.qty)`:                          "3",
		`toString(value.items)`:                        "[1,2]",
		`now()`:                                        "2026-01-02T03:04:05Z",
		`formatDate(value.ts, "date", "Asia/Tokyo")`:   "2026-03-04",
		`formatDate(1700000000000, "datetime")`:        "2023-11-14 22:13:20",
		`mask("12345678", "last4")`:                    "****5678",
		`len(hash("x"))`:                               int64(64),
		`lookup("countries", value.cc)`:                "Germany",
		`lookup("countries", "FR", "?")`:               "?",
		`lookup("nope", "FR")`:                         nil,
		`value.qty == 3 && value.first != "Bob"`:       true,
		`value.qty >= 4 || !(value.first == "Ada")`:    false,
		`"3" == value.qty`:                             true,
		`value.n == null`:                              true,
		`value.nested.a[0].b`:                          "deep",
		`topic`:                                        "orders",
		`'single \'quoted\''`:                          "single 'quoted'",
		`"tab\tnew\nline"`:                             "tab\tnew\nline",
		`"b" < "a"`:                                    false,
		`value.first < 3`:                              false,
	}
	c := formula.NewDefaultCompiler()
	for src, want := range cases {
		p, err := c.Compile(src)
		if err != nil {
			t.Errorf("%s: compile: %v", src, err)
			continue
		}
		got, err := p.Eval(evalCtx())
		if err != nil {
			t.Errorf("%s: eval: %v", src, err)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %#v want %#v", src, got, want)
		}
	}
}

func TestCompileErrors(t *testing.T) {
	cases := map[string]int{
		`concat(`:         7,
		`unknown(1)`:      0,
		`upper(1, 2)`:     0,
		`now(1)`:          0,
		`value..a`:        6,
		`foo.bar`:         0,
		`"open`:           0,
		`1 +`:             3,
		`value.a[x]`:      8,
		`1 2`:             2,
		`"bad \q escape"`: 7,
		`@`:               0,
		`value.a)`:        7,
		`concat(1,,2)`:    9,
	}
	c := formula.NewDefaultCompiler()
	for src, pos := range cases {
		_, err := c.Compile(src)
		var se *formula.SyntaxError
		if !errors.As(err, &se) {
			t.Errorf("%s: want syntax error, got %v", src, err)
			continue
		}
		if se.Pos != pos {
			t.Errorf("%s: pos %d want %d (%v)", src, se.Pos, pos, se)
		}
	}
}

func TestEvalErrors(t *testing.T) {
	c := formula.NewDefaultCompiler()
	for _, src := range []string{`toNumber("abc")`, `1 / 0`, `value.first * 2`, `formatDate("nope", "date")`, `formatDate(value.ts, "weird")`, `-value.first`} {
		p, err := c.Compile(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if _, err := p.Eval(evalCtx()); err == nil {
			t.Errorf("%s: want eval error", src)
		}
	}
}

func TestLimits(t *testing.T) {
	deep := ""
	for i := 0; i < 100; i++ {
		deep += "("
	}
	if _, err := formula.NewDefaultCompiler().Compile(deep + "1"); err == nil {
		t.Fatal("want depth error")
	}
	long := make([]byte, int(formula.LimitMaxLength)+1)
	for i := range long {
		long[i] = '1'
	}
	if _, err := formula.NewDefaultCompiler().Compile(string(long)); err == nil {
		t.Fatal("want length error")
	}
}

func TestCatalogue(t *testing.T) {
	names := map[formula.FuncName]bool{}
	for _, f := range formula.DefaultFunctionCatalog().All() {
		names[f.Name()] = true
		sig := f.Signature()
		if sig.Description == "" || sig.Example == "" || sig.Category == "" {
			t.Errorf("%s: incomplete signature", f.Name())
		}
		if _, err := formula.NewDefaultCompiler().Compile(sig.Example); err != nil {
			t.Errorf("%s example %q: %v", f.Name(), sig.Example, err)
		}
	}
	for _, n := range []formula.FuncName{formula.FuncConcat, formula.FuncUpper, formula.FuncLower, formula.FuncTrim, formula.FuncSubstring,
		formula.FuncLen, formula.FuncRound, formula.FuncCoalesce, formula.FuncIf, formula.FuncToNumber, formula.FuncToString,
		formula.FuncNow, formula.FuncFormatDate, formula.FuncMask, formula.FuncHash, formula.FuncLookup} {
		if !names[n] {
			t.Errorf("missing %s", n)
		}
	}
}

func FuzzFormulaParser(f *testing.F) {
	for _, s := range []string{`concat(value.a, " ", value.b)`, `round(value.x*1.1,2)`, `if(a>1,"x","y")`, `((((`, `"\`, `value.a[0].b`, `-!-1`, `lookup("t", key)`} {
		f.Add(s)
	}
	c := formula.NewDefaultCompiler()
	f.Fuzz(func(t *testing.T, src string) {
		p, err := c.Compile(src)
		if err != nil {
			var se *formula.SyntaxError
			if !errors.As(err, &se) {
				t.Fatalf("untyped compile error %v", err)
			}
			return
		}
		_, _ = p.Eval(evalCtx())
	})
}
