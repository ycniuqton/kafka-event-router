package quickjs

import (
	"testing"

	"github.com/eventrouter/router/pkg/sandbox"
)

func TestGuestSourceWrapping(t *testing.T) {
	gs, err := newGuestSource(sandbox.Source{Entry: sandbox.EntryPredicate, Code: "event.value.x > 1"})
	if err != nil || gs.entry.function != "__sb_predicate" || gs.position().lineOffset != 2 {
		t.Fatalf("predicate source = %+v, %v", gs, err)
	}
	if _, err := newGuestSource(sandbox.Source{Entry: "module", Code: "x"}); err == nil || err.Kind != sandbox.ErrorCompile {
		t.Fatalf("unknown entry err = %v", err)
	}
	empty, _ := newGuestSource(sandbox.Source{Entry: sandbox.EntryTransform, Code: ""})
	if empty.code == "" {
		t.Fatal("guest source must never be empty (empty means file input to the glue)")
	}
}

func TestStackPositionMapping(t *testing.T) {
	pos := stackPosition{lineOffset: 2, userLines: []string{"a > 1 &&"}}
	cases := []struct {
		stack     string
		line, col int
	}{
		{"    at __sb_predicate (step.js:3:5)\n", 1, 5},
		{"    at step.js:4:1\n", 1, 9},
		{"    at invoke (sandbox-prelude.js:10:2)\n", 0, 0},
		{"    at f (step.js:1:1)\n", 1, 1},
	}
	for _, tc := range cases {
		if l, c := pos.find(tc.stack); l != tc.line || c != tc.col {
			t.Errorf("%q -> %d:%d, want %d:%d", tc.stack, l, c, tc.line, tc.col)
		}
	}
}

func TestBudgetSplitsMemory(t *testing.T) {
	b := budgetFor(64 << 20)
	if b.heapLimit != 48<<20 || b.headroom != 16<<20 {
		t.Fatalf("budget = %+v", b)
	}
}
