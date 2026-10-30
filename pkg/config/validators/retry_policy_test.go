package validators_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
)

const retryRoute = `{"apiVersion":"router/v1","name":"r","source":{"connectionId":"src","topics":["t"],"format":{"type":"json"}},
 "branches":[{"id":"main","destination":{"connectionId":"src","topic":"out"}}],
 "errorPolicy":{"onError":"dlq","retry":RETRY}}`

func TestRetryPolicyBounds(t *testing.T) {
	cases := map[string]bool{
		`{"enabled":true}`: false,
		`{"enabled":true,"baseDelayMs":5000,"maxDelayMs":2000}`: true,
		`{"enabled":true,"baseDelayMs":600000}`:                 true,
		`{"enabled":true,"baseDelayMs":2000,"maxDelayMs":2000}`: false,
		`{"enabled":false,"maxAttempts":11}`:                    false,
		`{"enabled":true,"maxAttempts":0}`:                      false,
	}
	m := manager(t, engine.CompileOptions{Scripts: fakeScripts()})
	for retry, wantBound := range cases {
		doc := strings.Replace(retryRoute, "RETRY", retry, 1)
		is, err := m.Validate(context.Background(), config.StageDeploy, "team", []byte(doc))
		if err != nil {
			t.Fatal(err)
		}
		got := codesAt(is)["/errorPolicy/retry/maxDelayMs"] == config.IssueLimitOutOfRange
		if got != wantBound {
			t.Errorf("%s: bound issue %v, issues %+v", retry, got, is)
		}
	}
}

func TestRetryPolicyDefaults(t *testing.T) {
	p := config.RetryPolicyOf(nil)
	if p.Enabled || p.MaxAttempts != 5 || p.BaseDelay != time.Second || p.MaxDelay != 5*time.Minute {
		t.Fatalf("defaults %+v", p)
	}
	n, base := 3, 200
	p = config.RetryPolicyOf(&config.ErrorPolicy{Retry: &config.RetryPolicy{Enabled: true, MaxAttempts: &n, BaseDelayMs: &base}})
	if !p.Enabled || p.MaxAttempts != 3 || p.BaseDelay != 200*time.Millisecond || p.MaxDelay != 5*time.Minute {
		t.Fatalf("resolved %+v", p)
	}
}
