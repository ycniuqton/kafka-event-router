package password

import (
	"context"

	"github.com/eventrouter/router/internal/audit"
)

// Trail records password changes through the request's audit Scope, or directly for startup jobs.
type Trail struct {
	recorder audit.Recorder
}

// NewTrail builds the trail.
func NewTrail(recorder audit.Recorder) *Trail {
	return &Trail{recorder: recorder}
}

// Record writes c in the transaction carried by ctx.
func (t *Trail) Record(ctx context.Context, c audit.Change) error {
	if scope, ok := audit.FromContext(ctx); ok {
		return scope.Record(ctx, c)
	}
	return t.recorder.Record(ctx, audit.Entry{Actor: audit.Actor{Kind: audit.ActorSystem}, Action: c.Action,
		Object: c.Object, ObjectID: c.ObjectID})
}
