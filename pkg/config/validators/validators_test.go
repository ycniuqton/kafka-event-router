package validators_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/config/configtest"
	"github.com/eventrouter/router/pkg/config/validators"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/engine/scripttest"
)

const route = `{"apiVersion":"router/v1","name":"r","source":{"connectionId":"src","topics":["t"],"format":{"type":"json"}},
 "branches":[{"id":"main",
   "filter":{"type":"and","children":[{"type":"cond","field":"value.a","op":"regex","value":"("},{"type":"cond","field":"value..b","op":"eq","value":1},{"type":"expr","code":"bad js"}]},
   "transform":{"steps":[{"id":"j","kind":"js","code":"ok js"},{"id":"k","kind":"js","code":"broken"}]},
   "destination":{"connectionId":"DST","topic":"out"}}],
 "delivery":{"mode":"exactly_once"}}`

func fakeScripts() *scripttest.FuncRuntime {
	ok := func(context.Context, script.Invocation) (script.Result, error) { return script.Result{}, nil }
	return scripttest.NewFuncRuntime().Register("ok js", ok)
}

func manager(t *testing.T, opts engine.CompileOptions) *config.ValidatorManager {
	t.Helper()
	m, err := validators.NewDefaultManager(validators.Deps{Compiler: compile.NewDefaultCompiler(), Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func codesAt(is config.Issues) map[string]config.IssueCode {
	out := map[string]config.IssueCode{}
	for _, i := range is {
		out[string(i.Path)] = i.Code
	}
	return out
}

func TestDocOnlyValidatorsReportEveryIssueWithPointer(t *testing.T) {
	is, err := manager(t, engine.CompileOptions{Scripts: fakeScripts()}).Validate(context.Background(), config.StageDeploy, "team", []byte(route))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]config.IssueCode{
		"/branches/0/destination/connectionId": config.IssueExactlyOnceCrossCluster,
		"/branches/0/filter/children/0/value":  config.IssueRegexInvalid,
		"/branches/0/filter/children/1/field":  config.IssuePathInvalid,
		"/branches/0/filter/children/2/code":   config.IssueJSCompile,
		"/branches/0/transform/steps/1/code":   config.IssueJSCompile,
	}
	got := codesAt(is)
	for p, c := range want {
		if got[p] != c {
			t.Errorf("at %s want %s, got %s (all: %+v)", p, c, got[p], is)
		}
	}
	if len(is) != len(want) || !is.HasErrors() {
		t.Fatalf("issues: %+v", is)
	}
}

func TestSchemaFailureHaltsLaterValidators(t *testing.T) {
	bad := strings.Replace(route, `"op":"eq"`, `"op":"nope"`, 1)
	is, _ := manager(t, engine.CompileOptions{}).Validate(context.Background(), config.StageDraft, "", []byte(bad))
	for _, i := range is {
		if i.Code != config.IssueSchemaInvalid {
			t.Fatalf("later validator ran: %+v", is)
		}
	}
	if len(is) != 1 || is[0].Path != "/branches/0/filter/children/1/op" {
		t.Fatalf("issues %+v", is)
	}
	is, _ = manager(t, engine.CompileOptions{}).Validate(context.Background(), config.StageDraft, "", []byte(`{"x":`))
	if len(is) != 1 || is[0].Path != config.PointerRoot || is[0].Code != config.IssueSchemaInvalid {
		t.Fatalf("syntax issues %+v", is)
	}
}

func TestValidRouteHasNoIssuesAndDeliveryOK(t *testing.T) {
	raw, _ := os.ReadFile("../testdata/sysdesign_example.json")
	lib := configtest.NewInMemoryLibrary()
	opts := engine.CompileOptions{Library: lib, Scripts: scripttest.NewFuncRuntime().Register("function transform(event, ctx) { return event; }",
		func(context.Context, script.Invocation) (script.Result, error) { return script.Result{}, nil })}
	is, _ := manager(t, opts).Validate(context.Background(), config.StageDraft, "", raw)
	if len(is) != 1 || is[0].Code != config.IssueLibraryRefMissing || is[0].Path != "/branches/0/transform/steps/2/ref" {
		t.Fatalf("missing library version: issues %+v", is)
	}
	field, style := "value.customer.email", config.MaskStyleEmail
	lib.PutTransform("mask-customer-pii", 3, config.Steps{{Kind: config.StepKindMask, Field: &field, Style: &style}})
	if is, _ = manager(t, opts).Validate(context.Background(), config.StageDraft, "", raw); len(is) != 0 {
		t.Fatalf("pinned library version: issues %+v", is)
	}
	same := strings.Replace(strings.Replace(string(raw), `"conn_partner"`, `"conn_orders_prod"`, 1), `"at_least_once"`, `"exactly_once"`, 1)
	vc := &config.ValidationContext{Doc: mustParse(t, same)}
	if got := (validators.DeliveryModeValidator{}).Validate(context.Background(), vc); len(got) != 0 {
		t.Fatalf("same-connection exactly_once: %+v", got)
	}
}

// stubValidator emits fixed issues (draft vs deploy severity overrides).
type stubValidator struct {
	issues []config.Issue
}

func (stubValidator) Name() config.ValidatorName { return "stub" }
func (s stubValidator) Validate(context.Context, *config.ValidationContext) []config.Issue {
	return s.issues
}

func TestProfileSeverityOverridesPerStage(t *testing.T) {
	stub := stubValidator{issues: []config.Issue{config.NewError(config.IssueTestCaseFailed, "/tests/0", "failed")}}
	profile := config.DefaultValidationProfile()
	profile.Stages = map[config.ValidationStage][]config.ValidatorName{config.StageDraft: {"stub"}, config.StageDeploy: {"stub"}}
	m, err := config.NewValidatorManager(profile, config.NewValidatorRegistry(stub))
	if err != nil {
		t.Fatal(err)
	}
	draft, _ := m.Validate(context.Background(), config.StageDraft, "", []byte(`{}`))
	deploy, _ := m.Validate(context.Background(), config.StageDeploy, "", []byte(`{}`))
	if draft[0].Severity != config.SeverityWarning || deploy[0].Severity != config.SeverityError {
		t.Fatalf("draft %+v deploy %+v", draft, deploy)
	}
	if _, err := m.Validate(context.Background(), config.StageOffline, "", []byte(`{}`)); err == nil {
		t.Fatal("want unknown stage error")
	}
	if _, err := config.NewValidatorManager(config.DefaultValidationProfile(), config.NewValidatorRegistry()); err == nil {
		t.Fatal("want unknown validator error")
	}
}

func TestLibraryFake(t *testing.T) {
	lib := configtest.NewInMemoryLibrary()
	lib.PutTransform("mask", 3, config.Steps{{Kind: config.StepKindMask}})
	lib.PutLookupTable("countries", 1, config.LookupRows{{Key: "DE", Value: "Germany"}})
	if s, err := lib.Transform(context.Background(), "mask", 3); err != nil || len(s) != 1 {
		t.Fatal(err)
	}
	if _, err := lib.Transform(context.Background(), "mask", 4); err == nil {
		t.Fatal("want missing")
	}
	if r, err := lib.LookupTable(context.Background(), "countries", 1); err != nil || r[0].Key != "DE" {
		t.Fatal(err)
	}
	if _, err := lib.LookupTable(context.Background(), "x", 1); err == nil {
		t.Fatal("want missing")
	}
}

func mustParse(t *testing.T, raw string) *config.Document {
	t.Helper()
	d, err := config.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return d
}
