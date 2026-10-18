package rbac

import (
	"context"

	"github.com/eventrouter/router/internal/audit"
)

// AuditTrail records changes through the request's audit Scope, or directly when there is none (system jobs).
type AuditTrail struct {
	recorder audit.Recorder
}

// NewAuditTrail builds the trail.
func NewAuditTrail(recorder audit.Recorder) *AuditTrail {
	return &AuditTrail{recorder: recorder}
}

// Record writes c in the transaction carried by ctx.
func (t *AuditTrail) Record(ctx context.Context, c audit.Change) error {
	if scope, ok := audit.FromContext(ctx); ok {
		return scope.Record(ctx, c)
	}
	return t.recorder.Record(ctx, audit.Entry{Actor: audit.Actor{Kind: audit.ActorSystem}, Action: c.Action,
		Object: c.Object, ObjectID: c.ObjectID, TeamID: c.TeamID})
}
