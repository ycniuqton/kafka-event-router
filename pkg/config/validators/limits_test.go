package validators_test

import (
	"context"
	"testing"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
)

const limitsRoute = `{"apiVersion":"router/v1","name":"r","source":{"connectionId":"src","topics":["t"],"format":{"type":"json"}},
 "branches":[{"id":"main","destination":{"connectionId":"src","topic":"out"}}],
 "limits":{"inFlightMode":"auto","maxInFlight":100,"minInFlight":200,"jsMinInstances":4,"jsMaxInstances":2}}`

func TestLimitsValidatorReportsInvertedRanges(t *testing.T) {
	is, err := manager(t, engine.CompileOptions{Scripts: fakeScripts()}).Validate(context.Background(), config.StageDraft, "team", []byte(limitsRoute))
	if err != nil {
		t.Fatal(err)
	}
	got := codesAt(is)
	for _, p := range []config.Pointer{config.PointerMinInFlight, config.PointerJSMinInstances} {
		if got[string(p)] != config.IssueLimitOutOfRange {
			t.Fatalf("missing %s in %+v", p, is)
		}
	}
}

func TestSchemaRejectsUnknownInFlightMode(t *testing.T) {
	doc := `{"apiVersion":"router/v1","name":"r","source":{"connectionId":"src","topics":["t"],"format":{"type":"json"}},
 "branches":[{"id":"main","destination":{"connectionId":"src","topic":"out"}}],"limits":{"inFlightMode":"turbo"}}`
	is, err := manager(t, engine.CompileOptions{Scripts: fakeScripts()}).Validate(context.Background(), config.StageDraft, "team", []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(is) == 0 || is[0].Code != config.IssueSchemaInvalid {
		t.Fatalf("want schema_invalid, got %+v", is)
	}
}
