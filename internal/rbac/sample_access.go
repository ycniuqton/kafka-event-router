package rbac

import (
	"context"

	"github.com/eventrouter/router/internal/audit"
)

// SampleSurface names where event payloads are shown (each has its own audit action).
type SampleSurface string

const (
	SurfacePreview  SampleSurface = "preview"
	SurfaceLiveTail SampleSurface = "livetail"
	SurfaceDLQ      SampleSurface = "dlq"
)

// SampleDecision is what an owner (live-preview, monitoring, dead-letters) needs before showing payloads:
// whether to apply its type-preserving masker and which audit action to record (flows 5.7, FR-31).
type SampleDecision struct {
	Masked    bool
	Action    audit.Action
	Sensitive bool
}

// SampleAccess is the port payload owners call; it wraps Manager.SampleVisibility.
type SampleAccess interface {
	Decide(ctx context.Context, surface SampleSurface, connectionID string) (SampleDecision, error)
}

// SampleActions maps a surface and visibility to its catalogued audit action.
type SampleActions struct{}

// Of returns the action: preview loads are sample.viewed (masked) or sample.viewed_raw; tail and DLQ have one action each.
func (SampleActions) Of(surface SampleSurface, v Visibility) audit.Action {
	switch surface {
	case SurfaceLiveTail:
		return audit.ActionLiveTailOpened
	case SurfaceDLQ:
		return audit.ActionDLQViewed
	}
	if v == VisibilityRaw {
		return audit.ActionSampleViewedRaw
	}
	return audit.ActionSampleViewed
}

// ManagerSampleAccess implements SampleAccess over the Manager; samples without a connection (uploads) have
// no sensitivity, so only the caller's role decides (Viewers masked).
type ManagerSampleAccess struct {
	manager Manager
}

// NewSampleAccess builds the port.
func NewSampleAccess(m Manager) *ManagerSampleAccess {
	return &ManagerSampleAccess{manager: m}
}

// Decide implements SampleAccess; errors fail closed (masked), an unknown connection is ErrUnknownConnection.
func (a *ManagerSampleAccess) Decide(ctx context.Context, surface SampleSurface, connectionID string) (SampleDecision, error) {
	if connectionID == "" {
		return a.decision(surface, a.roleVisibility(ctx)), nil
	}
	v, err := a.manager.SampleVisibility(ctx, Resource{Type: ResourceConnection, ID: connectionID})
	if err != nil {
		v = VisibilityMasked
	}
	return a.decision(surface, v), err
}

// roleVisibility is raw when any of the caller's roles has sample.read_raw (editors and up), else masked.
func (a *ManagerSampleAccess) roleVisibility(ctx context.Context) Visibility {
	if a.manager.Decide(ctx, PermSampleReadRaw, Resource{Type: ResourceConnection}).Allowed {
		return VisibilityRaw
	}
	return VisibilityMasked
}

func (*ManagerSampleAccess) decision(surface SampleSurface, v Visibility) SampleDecision {
	masked := v == VisibilityMasked
	return SampleDecision{Masked: masked, Action: SampleActions{}.Of(surface, v), Sensitive: !masked}
}
