package routes

import (
	"context"

	"github.com/eventrouter/router/internal/audit"
)

// ActionRouteUnarchived is audited when an archived route comes back as paused.
const ActionRouteUnarchived audit.Action = "route.unarchived"

// ChangeUnarchived is the route_changed kind of an unarchive.
const ChangeUnarchived ChangeKind = "unarchived"

// Unarchive brings an archived route back as paused (desired_state archived → paused); its runners stay
// stopped until someone resumes it. Unarchiving a route that is not archived changes nothing (still audited).
func (m *RouteManager) Unarchive(ctx context.Context, id RouteID) (RouteDetail, error) {
	before, actor, err := m.loadWritable(ctx, id)
	if err != nil {
		return RouteDetail{}, err
	}
	if before.DesiredState != DesiredArchived {
		rec := AuditRecord{Action: ActionRouteUnarchived, Object: audit.ObjectRoute, ObjectID: string(id), Team: before.TeamID, Before: AuditViewOf(before), After: AuditViewOf(before)}
		if err := m.Auditor.Record(ctx, rec); err != nil {
			return RouteDetail{}, err
		}
		return m.detail(ctx, id)
	}
	var after Route
	err = m.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if after, err = m.Repos.Routes.SetDesiredState(ctx, id, DesiredPaused, actor.UserID); err != nil {
			return err
		}
		return m.Auditor.Record(ctx, AuditRecord{Action: ActionRouteUnarchived, Object: audit.ObjectRoute, ObjectID: string(id),
			Team: before.TeamID, Before: AuditViewOf(before), After: AuditViewOf(after)})
	})
	if err != nil {
		return RouteDetail{}, err
	}
	m.publish(ctx, ChangeOf(after, ChangeUnarchived))
	return m.detail(ctx, id)
}
