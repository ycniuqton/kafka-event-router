package mock_test

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/internal/assist/mock"
	"github.com/eventrouter/router/pkg/config"
)

var update = flag.Bool("update", false, "rewrite golden files")

var sampleFields = []string{"value.order_id", "value.cust_id", "value.customer.email", "value.customer.name", "value.card_number",
	"value.country", "value.currency", "value.items", "value.items[0].price", "value.status"}

func parse(t *testing.T, instruction string) []config.Step {
	t.Helper()
	return mock.NewMockIntentParser().Parse(instruction, mock.NewFieldResolver(sampleFields))
}

func TestIntentParserClauses(t *testing.T) {
	cases := []struct {
		instruction string
		kinds       []config.StepKind
		first       string
	}{
		{"hide the email", []config.StepKind{config.StepKindMask}, `"field":"value.customer.email"`},
		{"rename cust_id to customerId", []config.StepKind{config.StepKindRename}, `"to":"value.customerId"`},
		{"Rename customer.name to fullName", []config.StepKind{config.StepKindRename}, `"to":"value.customer.fullName"`},
		{"remove items and hash order id", []config.StepKind{config.StepKindRemove, config.StepKindHash}, `"fields":["value.items"]`},
		{"remove country, currency and status", []config.StepKind{config.StepKindRemove}, `"fields":["value.country","value.currency","value.status"]`},
		{"only keep order_id, status and currency", []config.StepKind{config.StepKindKeepOnly}, `"fields":["value.order_id","value.status","value.currency"]`},
		{"set status to \"done\"", []config.StepKind{config.StepKindSet}, `"value":"done"`},
		{"set the key to cust_id", []config.StepKind{config.StepKindSetKey}, `"path":"value.cust_id"`},
		{"use order_id as the key", []config.StepKind{config.StepKindSetKey}, `"path":"value.order_id"`},
		{"default country to \"DE\"", []config.StepKind{config.StepKindDefaultIfMissing}, `"value":"DE"`},
		{"mask card number", []config.StepKind{config.StepKindMask}, `"style":"last4"`},
		{"please make it nicer", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.instruction, func(t *testing.T) {
			steps := parse(t, tc.instruction)
			if len(steps) != len(tc.kinds) {
				t.Fatalf("got %d steps %+v, want kinds %v", len(steps), steps, tc.kinds)
			}
			for i, k := range tc.kinds {
				if steps[i].Kind != k {
					t.Errorf("step %d kind %s, want %s", i, steps[i].Kind, k)
				}
			}
			if tc.first != "" {
				raw, _ := json.Marshal(steps[0])
				if !contains(string(raw), tc.first) {
					t.Errorf("step %s does not contain %s", raw, tc.first)
				}
			}
		})
	}
}

func TestIntentParserKeepsInstructionOrder(t *testing.T) {
	steps := parse(t, "hash cust_id, then rename order_id to orderId and mask email")
	want := []config.StepKind{config.StepKindHash, config.StepKindRename, config.StepKindMask}
	for i, k := range want {
		if steps[i].Kind != k {
			t.Fatalf("order: got %v at %d, want %v", steps[i].Kind, i, k)
		}
	}
}

func TestFieldResolverFallsBackToValueRoot(t *testing.T) {
	r := mock.NewFieldResolver(nil)
	if got := r.Resolve("unknownField"); got != "value.unknownField" {
		t.Fatalf("got %q", got)
	}
	if got := r.Resolve("headers.trace"); got != "headers.trace" {
		t.Fatalf("rooted path changed: %q", got)
	}
}

// TestMockProposalsGolden pins the deterministic mock output for a set of instructions (TS-231).
func TestMockProposalsGolden(t *testing.T) {
	instructions := []string{
		"hide the email, rename cust_id to customerId",
		"only keep order_id and status",
		"remove country and currency then key by order_id",
		"mask card_number, hash customer.name, set status to 42",
	}
	p := mock.NewMockProvider()
	got := map[string]json.RawMessage{}
	for _, in := range instructions {
		c, err := p.Complete(context.Background(), assist.Prompt{Task: assist.TaskProposeSteps, Instruction: in, FieldSchema: sampleFields})
		if err != nil {
			t.Fatal(err)
		}
		got[in] = c.Steps
	}
	raw, _ := json.MarshalIndent(got, "", "  ")
	golden := filepath.Join("..", "testdata", "mock_proposals.golden.json")
	if *update {
		if err := os.WriteFile(golden, append(raw, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run with -update): %v", err)
	}
	if string(want) != string(raw)+"\n" {
		t.Fatalf("golden mismatch:\n got %s\nwant %s", raw, want)
	}
}

func TestMockExplainer(t *testing.T) {
	text := mock.NewMockExplainer().Explain(`function transform(event, ctx) { const v = event.value;
  return { ...event, value: { ...v, total: v.items.reduce((s, i) => s + i.price, 0), at: ctx.now() } }; }`)
	for _, want := range []string{"object spread", "reduce", "ctx.now"} {
		if !contains(text, want) {
			t.Errorf("explanation misses %q:\n%s", want, text)
		}
	}
	if got := mock.NewMockExplainer().Explain("  "); got != string(mock.TextEmpty) {
		t.Errorf("empty code: %q", got)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
