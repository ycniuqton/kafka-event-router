package approvals

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/rbac"
)

// RightsReader reads the caller's deploy permissions on a route's team.
type RightsReader struct {
	Access rbac.Manager
}

// Of evaluates route.deploy_nonprod, route.deploy_prod and route.write on the route.
func (r RightsReader) Of(ctx context.Context, route uuid.UUID, team rbac.TeamID) CallerRights {
	res := rbac.Resource{Type: rbac.ResourceRoute, ID: route.String(), TeamID: team}
	return CallerRights{
		DeployNonProd: r.Access.Decide(ctx, rbac.PermRouteDeployNonProd, res).Allowed,
		DeployProd:    r.Access.Decide(ctx, rbac.PermRouteDeployProd, res).Allowed,
		Write:         r.Access.Decide(ctx, rbac.PermRouteWrite, res).Allowed,
	}
}

// ResourceOf maps approvals to authorization targets.
type ResourceOf struct{}

// Deployment returns the approval's deployment on its route's team.
func (ResourceOf) Deployment(a Approval) rbac.Resource {
	return rbac.Resource{Type: rbac.ResourceDeployment, ID: a.ID.String(), TeamID: a.TeamID}
}

// ApprovalAudit renders approvals for audit entries.
type ApprovalAudit struct{}

// Of returns the audited fields (status, version, note, cancel reason).
func (ApprovalAudit) Of(a Approval, reason CancelReason) map[string]any {
	out := map[string]any{"routeId": a.RouteID, "versionNo": a.VersionNo, "status": a.Status}
	if a.DecisionNote != "" {
		out["decisionNote"] = a.DecisionNote
	}
	if reason != "" {
		out["cancelReason"] = reason
	}
	return out
}

// Recipients decides who gets an approval notification (bell): approvers on requests and withdrawals,
// the requester on decisions, requester and earlier commenters on comments; never the actor.
type Recipients struct {
	Store Store
}

// For returns the recipients of kind for a, excluding actor.
func (r Recipients) For(ctx context.Context, kind EventKind, a Approval, actor uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	var err error
	switch kind {
	case EventRequested, EventWithdrawn:
		ids, err = r.Store.Approvers(ctx, a.TeamID, actor)
	case EventCommented:
		ids, err = r.Store.Commenters(ctx, a.ID)
		ids = append(ids, r.requester(a)...)
	default:
		ids = r.requester(a)
	}
	if err != nil {
		return nil, err
	}
	return UserSet{}.Without(ids, actor), nil
}

func (Recipients) requester(a Approval) []uuid.UUID {
	if a.RequestedBy == nil {
		return nil
	}
	return []uuid.UUID{a.RequestedBy.ID}
}

// UserSet de-duplicates user id lists.
type UserSet struct{}

// Without returns the distinct ids of ids other than skip, in first-seen order.
func (UserSet) Without(ids []uuid.UUID, skip uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]bool{skip: true}
	out := []uuid.UUID{}
	for _, id := range ids {
		if !seen[id] {
			seen[id], out = true, append(out, id)
		}
	}
	return out
}
