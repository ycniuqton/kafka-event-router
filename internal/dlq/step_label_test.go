package dlq

import (
	"encoding/json"
	"testing"
)

func TestStepLabelsOfVersions(t *testing.T) {
	raw := json.RawMessage(`{"apiVersion":"router/v1","name":"r","source":{"connectionId":"c1","topics":["in"],"format":{"type":"json"}},
		"branches":[{"id":"main","transform":{"steps":[{"kind":"rename","from":"a","to":"b"},{"kind":"js","id":"enrich","code":"function transform(e){return e}"}]},
		"destination":{"connectionId":"c1","topic":"out"}}]}`)
	labels := StepLabelsOf{}.Versions([]VersionConfig{{ID: "v", No: 3, Config: raw}, {ID: "bad", No: 4, Config: json.RawMessage(`{`)}})
	cases := []struct {
		version      int
		branch, step string
		want         string
	}{
		{3, "main", "step-0", "Step 1 · Rename field"},
		{3, "main", "enrich", "Step 2 · JavaScript"},
		{3, "main", "filter", "Filter"},
		{4, "main", "step-0", "step-0"},
		{9, "other", "filter", "Filter"},
	}
	for _, c := range cases {
		if got := labels.Of(c.version, c.branch, c.step); got != c.want {
			t.Errorf("Of(%d,%s,%s) = %q, want %q", c.version, c.branch, c.step, got, c.want)
		}
	}
}
