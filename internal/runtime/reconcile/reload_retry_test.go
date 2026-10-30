package reconcile

import (
	"testing"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
	"github.com/eventrouter/router/pkg/config"
)

func TestRetryPolicyChangesRestartTheRunner(t *testing.T) {
	base := func(retry *config.RetryPolicy) *config.RouteConfig {
		return &config.RouteConfig{Name: "orders", Source: config.Source{ConnectionID: "c"},
			Branches:    []config.Branch{{ID: "main", Destination: config.Destination{ConnectionID: "c"}}},
			ErrorPolicy: &config.ErrorPolicy{OnError: config.ErrorActionDLQ, Retry: retry}}
	}
	c := ConfigReloadClassifier{DLQ: failures.DLQTargetResolver{Default: domain.DLQTargetSource}}
	three, base2 := 3, 2000
	if got := c.Classify(base(nil), base(&config.RetryPolicy{Enabled: true})); got != domain.ReloadFullRestart {
		t.Fatalf("enable: %s", got)
	}
	if got := c.Classify(base(&config.RetryPolicy{Enabled: true}), base(&config.RetryPolicy{Enabled: true, MaxAttempts: &three})); got != domain.ReloadFullRestart {
		t.Fatalf("attempts: %s", got)
	}
	if got := c.Classify(base(&config.RetryPolicy{Enabled: true}), base(&config.RetryPolicy{Enabled: true, BaseDelayMs: &base2})); got != domain.ReloadPipelineSwap {
		t.Fatalf("delay change: %s", got)
	}
	if got := c.Classify(base(nil), base(&config.RetryPolicy{Enabled: false})); got != domain.ReloadPipelineSwap {
		t.Fatalf("disabled: %s", got)
	}
}
