package runner_test

import (
	"context"
	"testing"

	"github.com/eventrouter/router/internal/runtime/runner"
)

// A failed start closes the consumer before launch; franz-go then fires the revoke callback on a runner
// without tracker, gate or committer. It must be a no-op instead of a nil-pointer panic.
func TestRebalanceCallbacksBeforeLaunchAreNoOps(t *testing.T) {
	r := &runner.RouteRunner{}
	tps := []runner.TopicPartition{{Topic: "orders", Partition: 0}}
	r.Revoked(context.Background(), tps)
	r.Lost(tps)
}
