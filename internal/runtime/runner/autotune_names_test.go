package runner_test

import (
	"testing"

	"github.com/eventrouter/router/internal/metrics/instruments"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/sandbox"
)

// The overview queries build names from instruments; the writers must export exactly those names.
func TestAutotuneSeriesNamesMatchTheQueryCatalog(t *testing.T) {
	pairs := map[string]string{
		string(runner.MetricInFlightLimit):  instruments.MetricInFlightLimit.Full(),
		string(sandbox.MetricPoolInstances): instruments.MetricJSPoolInstances.Full(),
		string(sandbox.MetricQueueWait):     instruments.MetricJSQueueWait.Full(),
	}
	for writer, query := range pairs {
		if writer != query {
			t.Errorf("writer exports %s, queries read %s", writer, query)
		}
	}
}
