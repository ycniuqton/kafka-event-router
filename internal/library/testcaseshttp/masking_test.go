package testcaseshttp

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/pkg/engine/trace"
)

type visibility bool

func (v visibility) RawAllowed(context.Context, string) bool { return bool(v) }

// TestViewerSeesMaskedCases: without raw access every event value, output and diff value is masked.
func TestViewerSeesMaskedCases(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	masking := CaseMasking{Visibility: visibility(false), Masker: preview.NewTypePreservingMasker()}
	mask := masking.For(r, "team")
	c := mask.Case(CaseDTO{InputEvent: json.RawMessage(`{"value":{"email":"ada@x.dev","n":5}}`), ExpectedOutput: json.RawMessage(`null`)})
	if strings.Contains(string(c.InputEvent), "ada@x.dev") || strings.Contains(string(c.InputEvent), "5") || string(c.ExpectedOutput) != "null" {
		t.Fatalf("masked case %s %s", c.InputEvent, c.ExpectedOutput)
	}
	rep := mask.Report(ReportDTO{Results: []ResultDTO{{ActualOutput: json.RawMessage(`{"value":{"s":"secret"}}`),
		Diff: []trace.PatchOp{{Op: "add", Path: "/value/s", Value: "secret", HasValue: true}}}}})
	if strings.Contains(string(rep.Results[0].ActualOutput), "secret") || rep.Results[0].Diff[0].Value == "secret" {
		t.Fatalf("masked report %+v", rep)
	}
	if (CaseMasking{Visibility: visibility(true), Masker: preview.NewTypePreservingMasker()}).For(r, "team") != nil {
		t.Fatal("raw access must not mask")
	}
}
