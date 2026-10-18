package routes_test

import (
	"encoding/json"
	"testing"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/validation"
)

// TestBranchSummaries (routes#p2-be-branches): every branch gets its own filter headline, transform and destination.
func TestBranchSummaries(t *testing.T) {
	doc := json.RawMessage(`{"apiVersion":"router/v1","name":"split","source":{"connectionId":"c1","topics":["o"],"format":{"type":"json"}},"branches":[
{"id":"eu","filter":{"type":"cond","field":"value.region","op":"eq","value":"EU"},"destination":{"connectionId":"c2","topic":"eu"}},
{"id":"rest","transform":{"steps":[{"kind":"js","code":"function transform(e){return e}"}]},"destination":{"connectionId":"c1","topic":"rest"}}]}`)
	cfg := routes.ConfigOf(doc)
	var s routes.RouteSummary
	routes.ConfigProjection{Names: map[string]validation.ConnectionInfo{"c2": {Name: "partner"}}}.Apply(&s, cfg)
	if len(s.Branches) != 2 || len(s.Destinations) != 2 || s.Transform != routes.TransformJS {
		t.Fatalf("%+v", s)
	}
	eu, rest := s.Branches[0], s.Branches[1]
	if eu.ID != "eu" || eu.FilterSummary.Headline != "region is EU" || eu.Transform != routes.TransformNone || eu.Destination.ConnectionName != "partner" {
		t.Fatalf("%+v", eu)
	}
	if rest.FilterSummary != nil || rest.Transform != routes.TransformJS || rest.Destination.Topic != "rest" {
		t.Fatalf("%+v", rest)
	}
	if s.FilterSummary != eu.FilterSummary {
		t.Fatal("the list headline is the first branch's")
	}
}
