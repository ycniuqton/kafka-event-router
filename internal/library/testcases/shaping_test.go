package testcases

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// shapedRoute keys by value.orderId, keeps only header "trace" and adds "source: router" at the destination.
const shapedRoute = `{"apiVersion":"router/v1","name":"shaped","source":{"connectionId":"a","topics":["in"],"format":{"type":"json"}},
"branches":[{"id":"main","transform":{"steps":[{"kind":"mask","field":"value.email","style":"email"}]},
"destination":{"connectionId":"b","topic":"out","key":{"mode":"field","field":"value.orderId"},
"headers":{"mode":"keep_selected","keep":["trace"],"add":[{"key":"source","value":"router"}]},"format":{"type":"json"}}}]}`

func shapedCase(expected string) TestCase {
	in := `{"key":"input-key","value":{"orderId":"ord-1","email":"ann@example.com"},"headers":[{"key":"trace","value":"t1"},{"key":"debug","value":"x"}]}`
	return TestCase{ID: uuid.New(), Name: "shaped", BranchID: "main", InputEvent: json.RawMessage(in), ExpectedOutcome: OutcomePassed, ExpectedOutput: json.RawMessage(expected)}
}

func TestRunnerComparesTheDestinationRecord(t *testing.T) {
	doc := RunTarget{TeamID: uuid.NewString(), RouteID: uuid.NewString(), RouteName: "shaped", Document: json.RawMessage(shapedRoute)}
	destination := `{"key":"ord-1","value":{"orderId":"ord-1","email":"a**@example.com"},"headers":{"trace":"t1","source":"router"}}`
	unshaped := `{"key":"input-key","value":{"orderId":"ord-1","email":"a**@example.com"},"headers":{"trace":"t1","debug":"x"}}`
	rep := newRunner().Run(context.Background(), doc, []TestCase{shapedCase(destination), shapedCase(unshaped)})
	if rep.Results[0].Result != ResultPass {
		t.Fatalf("destination-shaped expectation should pass: %+v (actual %s)", rep.Results[0], rep.Results[0].ActualOutput)
	}
	if rep.Results[1].Result != ResultFail || len(rep.Results[1].Diff) == 0 {
		t.Fatalf("pre-destination expectation should fail with a diff: %+v", rep.Results[1])
	}
}

func TestRecordBytesRender(t *testing.T) {
	cases := map[string]struct {
		in   []byte
		want string
	}{
		"absent": {nil, `null`},
		"json":   {[]byte(`{"a":1}`), `{"a":1}`},
		"text":   {[]byte("ord-1"), `"ord-1"`},
		"binary": {[]byte{0xff, 0xfe}, `"//4="`},
	}
	for name, c := range cases {
		got, _ := json.Marshal(RecordBytes{}.Render(c.in))
		if string(got) != c.want {
			t.Errorf("%s: got %s want %s", name, got, c.want)
		}
	}
}
