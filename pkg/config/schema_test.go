package config_test

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/config/validators"
)

func schemaIssues(t *testing.T, raw string) config.Issues {
	t.Helper()
	doc, err := config.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	vc := &config.ValidationContext{Stage: config.StageDraft, Doc: doc}
	return validators.MustSchemaValidator().Validate(context.Background(), vc)
}

func TestSchemaAcceptsSysDesignExample(t *testing.T) {
	raw, err := os.ReadFile("testdata/sysdesign_example.json")
	if err != nil {
		t.Fatal(err)
	}
	if is := schemaIssues(t, string(raw)); len(is) != 0 {
		t.Fatalf("unexpected issues: %+v", is)
	}
	doc, _ := config.Parse(raw)
	cfg, err := doc.Config()
	if err != nil || cfg.Branches[0].Transform.Steps[1].Kind != config.StepKindMask || cfg.DLQConnection() != "conn_orders_prod" {
		t.Fatalf("typed decode: %v %+v", err, cfg)
	}
}

func TestSchemaRejectsWithPointer(t *testing.T) {
	base, _ := os.ReadFile("testdata/sysdesign_example.json")
	cases := []struct {
		name, from, to string
		wantPath       config.Pointer
	}{
		{"unknown kind", `"kind": "mask"`, `"kind": "maskx"`, "/branches/0/transform/steps/1/kind"},
		{"unknown op", `"op": "gt"`, `"op": "greater"`, "/branches/0/filter/children/1/op"},
		{"unknown node type", `"type": "and"`, `"type": "xor"`, "/branches/0/filter/type"},
		{"unknown format", `"format": { "type": "json" },`, `"format": { "type": "xml" },`, "/source/format/type"},
		{"missing step member", `{ "kind": "rename", "from": "value.cust_id", "to": "value.customerId" }`, `{ "kind": "rename", "from": "value.cust_id" }`, "/branches/0/transform/steps/0"},
		{"limit out of range", `"jsTimeoutMs": 50`, `"jsTimeoutMs": 999999`, "/limits/jsTimeoutMs"},
		{"unknown member", `"name": "order-paid-to-partner",`, `"name": "order-paid-to-partner", "bogus": 1,`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := strings.Replace(string(base), c.from, c.to, 1)
			if raw == string(base) {
				t.Fatal("replacement did not apply")
			}
			is := schemaIssues(t, raw)
			for _, i := range is {
				if i.Path == c.wantPath && i.Code == config.IssueSchemaInvalid && i.Severity == config.SeverityError {
					return
				}
			}
			t.Fatalf("no issue at %q: %+v", c.wantPath, is)
		})
	}
}

func TestSchemaFilterNullPassesAll(t *testing.T) {
	raw := `{"apiVersion":"router/v1","name":"n","source":{"connectionId":"c","format":{"type":"json"}},
	  "branches":[{"id":"main","filter":null,"destination":{"connectionId":"c","topic":"t"}}],
	  "errorPolicy":{"onError":"dlq","dlqConnectionId":"dlq-conn"}}`
	if is := schemaIssues(t, raw); len(is) != 0 {
		t.Fatalf("issues: %+v", is)
	}
	doc, _ := config.Parse([]byte(raw))
	cfg, _ := doc.Config()
	if cfg.Branches[0].Filter != nil || cfg.DLQConnection() != "dlq-conn" {
		t.Fatalf("cfg %+v", cfg.Branches[0])
	}
}

func TestIssueCodeEnumMatchesSchema(t *testing.T) {
	raw, err := os.ReadFile("schema/engine-types.v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Defs map[string]struct {
			Enum []string `json:"enum"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	got := s.Defs["issueCode"].Enum
	want := []string{}
	for _, c := range allIssueCodes {
		want = append(want, string(c))
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schema issueCode enum %v != Go IssueCode %v", got, want)
	}
}

var allIssueCodes = []config.IssueCode{
	config.IssueSchemaInvalid, config.IssueConnectionNotFound, config.IssueConnectionForbidden, config.IssueFieldUnknown,
	config.IssueOperatorTypeMismatch, config.IssuePathInvalid, config.IssueRegexInvalid, config.IssueOperandInvalid,
	config.IssueFormulaInvalid, config.IssueJSCompile, config.IssueLibraryRefMissing, config.IssueLibraryUpdateAvailable,
	config.IssueTopicMissing, config.IssueExactlyOnceCrossCluster, config.IssueTestCaseFailed, config.IssueLimitOutOfRange,
	config.IssueStepUnsupported, config.IssueFormatUnsupported, config.IssueVersionUnsupported,
}

func TestDocumentParseAndLookup(t *testing.T) {
	if _, err := config.Parse([]byte(`{"a":1} x`)); err == nil {
		t.Fatal("want trailing data error")
	}
	if _, err := config.Parse([]byte(`{`)); err == nil {
		t.Fatal("want syntax error")
	}
	doc, _ := config.Parse([]byte(`{"a/b":[{"c~d":12345678901234567890}]}`))
	ptr := config.PointerRoot.Key("a/b").Index(0).Key("c~d")
	if ptr != "/a~1b/0/c~0d" {
		t.Fatalf("pointer %q", ptr)
	}
	v, ok := doc.Lookup(ptr)
	if !ok || v.(json.Number) != "12345678901234567890" {
		t.Fatalf("lookup %v %v", v, ok)
	}
	if _, ok := doc.Lookup("/a~1b/5"); ok {
		t.Fatal("out of range lookup")
	}
	if !reflect.DeepEqual(ptr.Tokens(), []string{"a/b", "0", "c~d"}) || config.PointerFromTokens(ptr.Tokens()) != ptr {
		t.Fatalf("tokens %v", ptr.Tokens())
	}
}

func TestIssuesSortMergeErrors(t *testing.T) {
	a := config.Issues{config.NewWarning(config.IssueFieldUnknown, "/b", "w"), config.NewError(config.IssuePathInvalid, "/a", "e")}
	b := config.Issues{config.NewError(config.IssueSchemaInvalid, "", "root")}
	m := a.Merge(b)
	if m[0].Path != "" || m[1].Path != "/a" || m[2].Path != "/b" || !m.HasErrors() || len(m.Errors()) != 2 {
		t.Fatalf("merge %+v", m)
	}
	if (config.Issues{config.NewWarning(config.IssueFieldUnknown, "/x", "w")}).HasErrors() {
		t.Fatal("warnings only")
	}
}

func TestLimitsResolvedAndUpgrader(t *testing.T) {
	var nilLimits *config.Limits
	if r := nilLimits.Resolved(); r.MaxOutputsPerEvent != int(config.LimitDefaultMaxOutputsPerEvent) || r.JSTimeoutMs != int(config.LimitDefaultJSTimeoutMs) {
		t.Fatalf("defaults %+v", r)
	}
	five := 5
	if r := (&config.Limits{MaxOutputsPerEvent: &five}).Resolved(); r.MaxOutputsPerEvent != 5 || r.MaxOutputBytes != int(config.LimitDefaultMaxOutputBytes) {
		t.Fatalf("override %+v", r)
	}
	m := config.DefaultUpgraderManager()
	if _, err := m.ForVersion("router/v9"); err == nil {
		t.Fatal("want unsupported version")
	}
	u, err := m.ForVersion(config.APIVersionRouterV1)
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := config.Parse([]byte(`{}`))
	if got, _ := u.Upgrade(doc); got != doc {
		t.Fatal("identity upgrade")
	}
}
