package gateway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/internal/preview/gateway"
)

func TestDemoLoadWithoutConnectionNeedsOnlyRead(t *testing.T) {
	f := newFixture(t, viewerAuthz{denyConnection: true})
	rec, body := f.do(http.MethodPost, "/api/v1/preview/samples", map[string]any{"mode": "demo", "count": 20})
	if rec.Code != http.StatusCreated || len(f.calls) != 1 {
		t.Fatalf("status %d body %v calls %d", rec.Code, body, len(f.calls))
	}
	if len(f.recorder.entries) != 1 {
		t.Fatalf("audit entries = %+v", f.recorder.entries)
	}
	var diff map[string]any
	_ = json.Unmarshal(f.recorder.entries[0].Diff, &diff)
	after, _ := diff["after"].(map[string]any)
	if f.recorder.entries[0].Object != audit.ObjectSampleSession || !strings.Contains(string(f.recorder.entries[0].Diff), `"demo"`) {
		t.Fatalf("audit entry = %+v (after %v)", f.recorder.entries[0], after)
	}
}

func TestDemoLoadOnConnectionChecksTheConnection(t *testing.T) {
	f := newFixture(t, viewerAuthz{denyConnection: true})
	rec, body := f.do(http.MethodPost, "/api/v1/preview/samples", map[string]any{"mode": "demo", "connectionId": "c1", "topics": []string{"order-events"}})
	if rec.Code != http.StatusForbidden || body["path"] != "/connectionId" || len(f.calls) != 0 {
		t.Fatalf("status %d body %v", rec.Code, body)
	}
}

func TestAssistSampleMaskerFollowsSampleVisibility(t *testing.T) {
	m := gateway.AssistSampleMasker{
		Authorizer: gateway.PreviewAuthorizer{Authz: viewerAuthz{}, Samples: sensitiveSamples{}},
		Values:     preview.NewSampleValueMasker(),
	}
	p := auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New()}
	samples := []json.RawMessage{json.RawMessage(`{"email":"jane@example.com","amount":42.5,"vip":true}`)}
	open := m.Mask(context.Background(), p, "plain", samples)
	if open.Masked || string(open.Samples[0]) != string(samples[0]) {
		t.Fatalf("plain connection = %+v", open)
	}
	for _, conn := range []string{"sensitive", "broken"} {
		got := m.Mask(context.Background(), p, conn, samples)
		text := string(got.Samples[0])
		if !got.Masked || strings.Contains(text, "jane") || strings.Contains(text, "42.5") {
			t.Fatalf("%s: samples = %s", conn, text)
		}
	}
}
