package testcases

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/enginetest"
)

const (
	filterNotSkip = `{"type":"cond","field":"value.skip","op":"not_exists"}`
	recipeSteps   = `[{"kind":"rename","from":"value.a","to":"value.b"},
{"kind":"drop_if","condition":{"type":"cond","field":"value.drop","op":"eq","value":true}},
{"kind":"convert_type","field":"value.n","targetType":"number","onFailure":"error"}]`
)

func newRunner() *TestCaseRunner {
	return NewTestCaseRunner(RunnerDeps{Compiler: compile.NewDefaultCompiler(), Comparators: DefaultComparatorManager()})
}

func target(filter, steps string) RunTarget {
	return RunTarget{TeamID: uuid.NewString(), RouteID: uuid.NewString(), RouteName: "test", Document: json.RawMessage(enginetest.RouteJSON(filter, steps))}
}

func tc(name, value string, outcome Outcome, output string) TestCase {
	return TestCase{ID: uuid.New(), Name: name, BranchID: "main", InputEvent: json.RawMessage(`{"key":null,"value":` + value + `,"headers":[]}`),
		ExpectedOutcome: outcome, ExpectedOutput: json.RawMessage(output)}
}

func TestRunnerOutcomes(t *testing.T) {
	cases := []struct {
		tc     TestCase
		result CaseResult
		actual Outcome
	}{
		{tc("passed", `{"a":1,"n":"5"}`, OutcomePassed, `{"key":null,"value":{"b":1,"n":5},"headers":{}}`), ResultPass, OutcomePassed},
		{tc("filtered", `{"skip":1}`, OutcomeFiltered, `null`), ResultPass, OutcomeFiltered},
		{tc("dropped", `{"drop":true}`, OutcomeDropped, `null`), ResultPass, OutcomeDropped},
		{tc("error", `{"n":"x"}`, OutcomeError, `null`), ResultPass, OutcomeError},
		{tc("changed output", `{"a":2,"n":"5"}`, OutcomePassed, `{"key":null,"value":{"b":1,"n":5},"headers":{}}`), ResultFail, OutcomePassed},
		{tc("now dropped", `{"drop":true}`, OutcomePassed, `{"key":null,"value":{},"headers":{}}`), ResultFail, OutcomeDropped},
	}
	all := []TestCase{}
	for _, c := range cases {
		all = append(all, c.tc)
	}
	rep := newRunner().Run(context.Background(), target(filterNotSkip, recipeSteps), all)
	if len(rep.Results) != len(cases) || rep.Passed != 4 || rep.Failed != 2 {
		t.Fatalf("report %+v", rep)
	}
	for i, c := range cases {
		r := rep.Results[i]
		if r.Result != c.result || r.ActualOutcome != c.actual {
			t.Errorf("%s: result=%s actual=%s err=%s", c.tc.Name, r.Result, r.ActualOutcome, r.Error)
		}
	}
	if len(rep.Results[4].Diff) != 1 || rep.Results[4].Diff[0].Path != "/value/b" {
		t.Errorf("diff %+v", rep.Results[4].Diff)
	}
}

func TestRunnerCompileErrorAndUnknownBranch(t *testing.T) {
	bad := `[{"kind":"js","code":"function transform(e){return e}"}]`
	rep := newRunner().Run(context.Background(), target("null", bad), []TestCase{tc("js", `{}`, OutcomePassed, `{}`)})
	if rep.Results[0].Result != ResultError || rep.Results[0].Error == "" {
		t.Fatalf("want compile error, got %+v", rep.Results[0])
	}
	other := tc("other", `{}`, OutcomePassed, `{}`)
	other.BranchID = "missing"
	rep = newRunner().Run(context.Background(), target("null", `[]`), []TestCase{other})
	if rep.Results[0].Result != ResultError || rep.Results[0].Error != string(MsgBranchMissing) {
		t.Fatalf("want missing branch, got %+v", rep.Results[0])
	}
}

func TestRunnerUndecodableInput(t *testing.T) {
	c := tc("bad", `{}`, OutcomePassed, `{}`)
	c.InputEvent = json.RawMessage(`{"headers":"nope"}`)
	rep := newRunner().Run(context.Background(), target("null", `[]`), []TestCase{c})
	if rep.Results[0].Result != ResultError {
		t.Fatalf("got %+v", rep.Results[0])
	}
}

func TestStepsMergerReplacesFirstBranchSteps(t *testing.T) {
	doc := json.RawMessage(enginetest.RouteJSON("null", `[{"kind":"remove","fields":["value.x"]}]`))
	out, err := StepsMerger{}.Merge(doc, json.RawMessage(`[{"kind":"hash","field":"value.y"}]`))
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Branches []struct {
			Transform struct {
				Steps []map[string]any `json:"steps"`
			} `json:"transform"`
		} `json:"branches"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil || parsed.Branches[0].Transform.Steps[0]["kind"] != "hash" {
		t.Fatalf("merged %s", out)
	}
}
