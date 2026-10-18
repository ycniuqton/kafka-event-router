package testcases

import (
	"encoding/json"
	"testing"
)

func TestExactJSONComparator(t *testing.T) {
	cases := []struct {
		name      string
		want, got Observation
		equal     bool
		diffOps   int
	}{
		{"key order ignored", obs(OutcomePassed, `{"value":{"a":1,"b":2}}`), obs(OutcomePassed, `{"value":{"b":2,"a":1}}`), true, 0},
		{"numbers by value", obs(OutcomePassed, `{"value":{"n":1}}`), obs(OutcomePassed, `{"value":{"n":1.0}}`), true, 0},
		{"null vs missing", obs(OutcomePassed, `{"value":{"n":null}}`), obs(OutcomePassed, `{"value":{}}`), false, 1},
		{"changed value", obs(OutcomePassed, `{"value":{"n":1}}`), obs(OutcomePassed, `{"value":{"n":2}}`), false, 1},
		{"added member", obs(OutcomePassed, `{"value":{}}`), obs(OutcomePassed, `{"value":{"x":"y"}}`), false, 1},
		{"outcome differs", obs(OutcomePassed, `{"value":{}}`), obs(OutcomeDropped, `null`), false, 0},
		{"filtered equal", obs(OutcomeFiltered, `null`), obs(OutcomeFiltered, `null`), true, 0},
		{"dropped ignores output", obs(OutcomeDropped, `null`), obs(OutcomeDropped, `{"x":1}`), true, 0},
	}
	m := DefaultComparatorManager()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := m.Compare(c.want, c.got)
			if got.Equal != c.equal || len(got.Diff) != c.diffOps {
				t.Fatalf("equal=%v diff=%v", got.Equal, got.Diff)
			}
			if !got.Equal && got.Message == "" {
				t.Fatal("missing message")
			}
		})
	}
}

func TestComparatorDiffPaths(t *testing.T) {
	got := ExactJSONComparator{}.Compare(obs(OutcomePassed, `{"value":{"a":1,"gone":true}}`), obs(OutcomePassed, `{"value":{"a":2}}`))
	paths := map[string]string{}
	for _, op := range got.Diff {
		paths[op.Path] = op.Op
	}
	if paths["/value/a"] != "replace" || paths["/value/gone"] != "remove" {
		t.Fatalf("diff %+v", got.Diff)
	}
}

func TestComparatorManagerRejectsUnknownKind(t *testing.T) {
	if _, err := NewComparatorManager("ignore_paths", ExactJSONComparator{}); err == nil {
		t.Fatal("want error")
	}
}

func obs(o Outcome, raw string) Observation {
	return Observation{Outcome: o, Output: json.RawMessage(raw)}
}
