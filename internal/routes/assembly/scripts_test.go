package assembly

import (
	"context"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/routes/validation"
	"github.com/eventrouter/router/pkg/config"
)

type noConnections struct{}

func (noConnections) Find(context.Context, []string) (map[string]validation.ConnectionInfo, error) {
	return map[string]validation.ConnectionInfo{"c1": {ID: "c1"}, "c2": {ID: "c2"}}, nil
}

type noNames struct{}

func (noNames) NameTakenByOther(context.Context, string, string, string) (bool, error) {
	return false, nil
}

func TestSandboxCheckerReportsJSCompileErrors(t *testing.T) {
	ctx := context.Background()
	checker, err := NewSandboxScriptChecker(ctx)
	if err != nil {
		t.Skipf("sandbox unavailable: %v", err)
	}
	defer checker.Close(ctx)
	m, err := ValidatorFactory{Connections: noConnections{}, Names: noNames{}, Scripts: checker}.Build(Settings{})
	if err != nil {
		t.Fatal(err)
	}
	doc := func(code string) []byte {
		return []byte(`{"apiVersion":"router/v1","name":"r","source":{"connectionId":"c1","topics":["t"],"format":{"type":"json"}},
			"branches":[{"id":"main","transform":{"steps":[{"kind":"js","code":` + code + `}]},"destination":{"connectionId":"c2","topic":"o"}}]}`)
	}
	ok := m.Run(ctx, validation.Request{Mode: validation.ModeDeploy, TeamID: "t", Raw: doc(`"function transform(event, ctx) { return event; }"`)})
	if !ok.Valid {
		t.Fatalf("%+v", ok.Issues)
	}
	bad := m.Run(ctx, validation.Request{Mode: validation.ModeDeploy, TeamID: "t", Raw: doc(`"function transform(event, ctx) { return event"`)})
	if bad.Valid || bad.Issues[0].Code != config.IssueJSCompile || !strings.HasPrefix(string(bad.Issues[0].Path), "/branches/0/transform/steps/0") {
		t.Fatalf("%+v", bad.Issues)
	}
}
