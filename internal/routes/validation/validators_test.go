package validation

import (
	"context"
	"errors"
	"testing"

	"github.com/eventrouter/router/pkg/config"
)

type fakeLookup map[string]ConnectionInfo

func (f fakeLookup) Find(_ context.Context, ids []string) (map[string]ConnectionInfo, error) {
	out := map[string]ConnectionInfo{}
	for _, id := range ids {
		if c, ok := f[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

type fakeGrants map[string]bool

func (f fakeGrants) CanUseConnection(_ context.Context, _, id string) (bool, error) {
	return f[id], nil
}

type fakeTopics struct {
	topics map[string]bool
	err    error
}

func (f fakeTopics) TopicExists(_ context.Context, conn, topic string) (bool, error) {
	return f.topics[conn+"/"+topic], f.err
}

type fakeSchemas struct {
	schema FieldSchema
	ok     bool
}

func (f fakeSchemas) LatestSchema(context.Context, SchemaRequest) (FieldSchema, bool, error) {
	return f.schema, f.ok, nil
}

type fakeNames map[string]bool

func (f fakeNames) NameTakenByOther(_ context.Context, _, name, _ string) (bool, error) {
	return f[name], nil
}

const docWithDLQ = `{"apiVersion":"router/v1","name":"r","source":{"connectionId":"c1","topics":["t"],"format":{"type":"json"}},
"branches":[{"id":"main","filter":{"type":"and","children":[
  {"type":"cond","field":"value.amount","op":"gt","value":1},
  {"type":"cond","field":"value.typo","op":"eq","value":"x"},
  {"type":"not","child":{"type":"cond","field":"value.name","op":"starts_with","value":"a"}},
  {"type":"cond","field":"value.missing","op":"exists"}]},
 "destination":{"connectionId":"c2","topic":"out"}},
 {"id":"b2","destination":{"connectionId":"c3","topic":"new","createTopic":{"partitions":3}}}],
"errorPolicy":{"onError":"dlq","dlqConnectionId":"c9"}}`

func input(t *testing.T, mode ValidationMode, raw string) ValidationInput {
	t.Helper()
	in := InputBuilder{}.Build(Request{Mode: mode, TeamID: "team-a", RouteID: "r1", RouteName: "r", Raw: []byte(raw)})
	if in.Config == nil {
		t.Fatal("document did not decode")
	}
	return in
}

func codes(is []config.Issue) map[string]config.IssueCode {
	out := map[string]config.IssueCode{}
	for _, i := range is {
		out[string(i.Path)] = i.Code
	}
	return out
}

func TestConnectionAccessValidator(t *testing.T) {
	v := ConnectionAccessValidator{
		Lookup: fakeLookup{"c1": {ID: "c1", TeamID: "team-a"}, "c2": {ID: "c2", TeamID: "team-b", Name: "partner"}, "c3": {ID: "c3", TeamID: "team-b"}},
		Grants: fakeGrants{"c3": true},
	}
	is, err := v.Validate(context.Background(), input(t, ModeDeploy, docWithDLQ))
	if err != nil {
		t.Fatal(err)
	}
	got := codes(is)
	if len(got) != 2 || got["/branches/0/destination/connectionId"] != config.IssueConnectionForbidden || got["/errorPolicy/dlqConnectionId"] != config.IssueConnectionNotFound {
		t.Fatalf("%+v", is)
	}
	v.Grants = nil
	is, _ = v.Validate(context.Background(), input(t, ModeDeploy, docWithDLQ))
	if len(is) != 1 {
		t.Fatalf("without grants only existence is checked: %+v", is)
	}
}

func TestDestinationTopicValidator(t *testing.T) {
	v := DestinationTopicValidator{Inspector: fakeTopics{topics: map[string]bool{}}}
	is, _ := v.Validate(context.Background(), input(t, ModeDeploy, docWithDLQ))
	if len(is) != 1 || is[0].Code != config.IssueTopicMissing || is[0].Path != "/branches/0/destination/topic" {
		t.Fatalf("%+v", is)
	}
	v.Inspector = fakeTopics{topics: map[string]bool{"c2/out": true}}
	if is, _ := v.Validate(context.Background(), input(t, ModeDeploy, docWithDLQ)); len(is) != 0 {
		t.Fatalf("%+v", is)
	}
	v.Inspector = fakeTopics{err: errors.New("unreachable")}
	is, _ = v.Validate(context.Background(), input(t, ModeDeploy, docWithDLQ))
	if len(is) != 1 || is[0].Severity != config.SeverityWarning {
		t.Fatalf("%+v", is)
	}
}

func TestFieldTypeValidator(t *testing.T) {
	v := FieldTypeValidator{Schemas: fakeSchemas{ok: true, schema: FieldSchema{"value.amount": FieldNumber, "value.name": FieldNumber}}}
	is, _ := v.Validate(context.Background(), input(t, ModeDraft, docWithDLQ))
	got := codes(is)
	if len(got) != 2 ||
		got["/branches/0/filter/children/1/field"] != config.IssueFieldUnknown ||
		got["/branches/0/filter/children/2/child/op"] != config.IssueOperatorTypeMismatch {
		t.Fatalf("%+v", is)
	}
	for _, i := range is {
		if i.Severity != config.SeverityWarning {
			t.Fatal("field type issues are warnings")
		}
	}
	v.Schemas = fakeSchemas{ok: false}
	if is, _ := v.Validate(context.Background(), input(t, ModeDraft, docWithDLQ)); len(is) != 0 {
		t.Fatal("no samples → skip")
	}
}

func TestUniqueNameValidator(t *testing.T) {
	v := UniqueNameValidator{Index: fakeNames{"r": true}}
	is, _ := v.Validate(context.Background(), input(t, ModeImport, docWithDLQ))
	if len(is) != 1 || is[0].Code != IssueNameTaken {
		t.Fatalf("%+v", is)
	}
	in := input(t, ModeDraft, docWithDLQ)
	in.RouteName = "other"
	is, _ = v.Validate(context.Background(), in)
	if len(is) != 1 || is[0].Code != IssueNameMismatch || is[0].Severity != config.SeverityWarning {
		t.Fatalf("%+v", is)
	}
}

type fakeCases struct {
	results []TestCaseResult
	err     error
}

func (f fakeCases) Run(context.Context, string, []byte) ([]TestCaseResult, error) {
	return f.results, f.err
}

// TestTestCaseValidator (routes#p2-be-testcase-validator): failing cases block deploys, warn on draft and rollback.
func TestTestCaseValidator(t *testing.T) {
	v := TestCaseValidator{Runner: fakeCases{results: []TestCaseResult{{Name: "paid", Passed: true}, {Name: "refund", Message: "dropped"}}}}
	want := map[ValidationMode]config.Severity{ModeDraft: config.SeverityWarning, ModeDeploy: config.SeverityError, ModeRollback: config.SeverityWarning}
	for mode, sev := range want {
		is, err := v.Validate(context.Background(), input(t, mode, docWithDLQ))
		if err != nil || len(is) != 1 || is[0].Severity != sev || is[0].Code != config.IssueTestCaseFailed || is[0].Path != "/branches/0/transform" {
			t.Fatalf("%s: %+v %v", mode, is, err)
		}
	}
	in := input(t, ModeDeploy, docWithDLQ)
	in.RouteID = ""
	if is, _ := v.Validate(context.Background(), in); len(is) != 0 {
		t.Fatal("new routes have no saved cases")
	}
	if _, err := (TestCaseValidator{Runner: fakeCases{err: errors.New("down")}}).Validate(context.Background(), in); err != nil {
		t.Fatal("route id empty: runner not called")
	}
	if ModeList(v.Modes()).Contains(ModeImport) {
		t.Fatal("imports have no saved cases")
	}
}

// TestSchemaRegistryValidator (routes#p2-be-formats): Avro/Protobuf need a registry on that connection.
func TestSchemaRegistryValidator(t *testing.T) {
	raw := `{"apiVersion":"router/v1","name":"r","source":{"connectionId":"c1","topics":["t"],"format":{"type":"avro"},"keyFormat":{"type":"string"}},
"branches":[{"id":"a","destination":{"connectionId":"c2","topic":"o","format":{"type":"protobuf"}}},
{"id":"b","destination":{"connectionId":"c1","topic":"p","format":{"type":"json"}}}]}`
	v := SchemaRegistryValidator{Lookup: fakeLookup{"c1": {ID: "c1", Name: "plain"}, "c2": {ID: "c2", Name: "sr", HasSchemaRegistry: true}}}
	is, err := v.Validate(context.Background(), input(t, ModeDeploy, raw))
	got := codes(is)
	if err != nil || len(is) != 2 || got["/source/format"] != IssueSchemaRegistryMissing || got["/branches/0/destination/format"] != IssueMessageTypeDefault {
		t.Fatalf("%+v %v", is, err)
	}
	if is[1].Severity != config.SeverityWarning && is[0].Severity != config.SeverityWarning {
		t.Fatal("message type default is only a warning")
	}
}

// TestSourceStartValidator (routes#p2-be-formats): topic pattern and offsets / timestamp start modes.
func TestSourceStartValidator(t *testing.T) {
	cases := map[string]struct {
		source string
		mode   ValidationMode
		want   map[string]config.IssueCode
	}{
		"pattern ok":     {`"topicPattern":"^orders\\..*$"`, ModeDeploy, map[string]config.IssueCode{}},
		"bad pattern":    {`"topicPattern":"(["`, ModeDeploy, map[string]config.IssueCode{"/source/topicPattern": config.IssueRegexInvalid}},
		"both":           {`"topics":["a"],"topicPattern":"^a$"`, ModeDraft, map[string]config.IssueCode{"/source/topicPattern": IssueTopicsAndPattern}},
		"none on deploy": {`"topics":[]`, ModeDeploy, map[string]config.IssueCode{"/source/topics": IssueNoTopics}},
		"offsets":        {`"topics":["a"],"startFrom":{"mode":"offsets","offsets":{"0":5,"x":1}}`, ModeDeploy, map[string]config.IssueCode{"/source/startFrom/offsets/x": IssueOffsetsInvalid}},
		"offsets empty":  {`"topics":["a"],"startFrom":{"mode":"offsets"}`, ModeDeploy, map[string]config.IssueCode{"/source/startFrom/offsets": IssueOffsetsInvalid}},
		"timestamp":      {`"topics":["a"],"startFrom":{"mode":"timestamp"}`, ModeDeploy, map[string]config.IssueCode{"/source/startFrom/at": IssueStartAtMissing}},
	}
	for name, c := range cases {
		raw := `{"apiVersion":"router/v1","name":"r","source":{"connectionId":"c1",` + c.source + `,"format":{"type":"json"}},"branches":[{"id":"main","destination":{"connectionId":"c2","topic":"o"}}]}`
		is, _ := SourceStartValidator{}.Validate(context.Background(), input(t, c.mode, raw))
		got := codes(is)
		if len(got) != len(c.want) {
			t.Errorf("%s: %+v", name, is)
		}
		for path, code := range c.want {
			if got[path] != code {
				t.Errorf("%s: %s = %s, want %s", name, path, got[path], code)
			}
		}
	}
	draft, _ := SourceStartValidator{}.Validate(context.Background(), input(t, ModeDraft, `{"apiVersion":"router/v1","name":"r","source":{"connectionId":"c1","format":{"type":"json"}},"branches":[{"id":"main","destination":{"connectionId":"c2","topic":"o"}}]}`))
	if len(draft) != 1 || draft[0].Severity != config.SeverityWarning {
		t.Fatalf("drafts may lack topics: %+v", draft)
	}
}
