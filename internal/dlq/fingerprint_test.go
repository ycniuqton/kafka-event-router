package dlq

import (
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/dlq/record"
)

func TestMessageNormalizerPatterns(t *testing.T) {
	cases := map[string]string{
		"Cannot read properties of undefined (reading 'reduce')":     "Cannot read properties of undefined (reading 'reduce')",
		"customer ord_123 not found":                                 "customer <id> not found",
		"customer usr-9f2a not found":                                "customer <id> not found",
		"order 3f9ac1d2-0b4e-4c1a-9e1f-1234567890ab rejected":        "order <uuid> rejected",
		"hash 9f86d081884c7d65 mismatch":                             "hash <hex> mismatch",
		"read offset 884162 on partition 3":                          "read offset <n> on partition <n>",
		"TypeError at transform (step-1.js:7:22)":                    "TypeError at transform (<id>.js:<n>:<n>)",
		"deadline 2026-10-13T09:41:07.123Z exceeded after 1500.5 ms": "deadline <ts> exceeded after <n> ms",
		"value  too\tlong:   4096 > 2048":                            "value too long: <n> > <n>",
		"unexpected token } in JSON at position 15":                  "unexpected token } in JSON at position <n>",
		"deadbeef is a word":                                         "deadbeef is a word",
	}
	n := NewMessageNormalizer()
	for in, want := range cases {
		if got := n.Pattern(in); got != want {
			t.Errorf("%q:\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestMessageNormalizerCutsLongPatterns(t *testing.T) {
	got := NewMessageNormalizer().Pattern(strings.Repeat("é", 500))
	if len([]rune(got)) != int(MaxPatternRunes) {
		t.Fatalf("got %d runes", len([]rune(got)))
	}
}

func TestFingerprintGroupsTheSameCause(t *testing.T) {
	f := NewFingerprinter()
	base := record.Failure{Step: "js1", ErrorType: record.ErrorStep, Message: "customer ord_1 has no items at offset 1"}
	other := base
	other.Message, other.VersionNo, other.Branch = "customer ord_99872 has no items at offset 4411", 7, "b2"
	a, b := f.Of("r1", base), f.Of("r1", other)
	if a.ID != b.ID || a.Pattern != b.Pattern || len(a.ID) != int(FingerprintHex) {
		t.Fatalf("same cause split: %+v %+v", a, b)
	}
	diffs := []struct {
		route string
		f     record.Failure
	}{
		{"r2", base},
		{"r1", record.Failure{Step: "js2", ErrorType: base.ErrorType, Message: base.Message}},
		{"r1", record.Failure{Step: base.Step, ErrorType: record.ErrorTimeout, Message: base.Message}},
		{"r1", record.Failure{Step: base.Step, ErrorType: base.ErrorType, Message: "another message"}},
	}
	for _, d := range diffs {
		if f.Of(d.route, d.f).ID == a.ID {
			t.Errorf("%s %+v should not share fingerprint", d.route, d.f)
		}
	}
}
