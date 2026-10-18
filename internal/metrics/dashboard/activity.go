package dashboard

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
)

// ActivityVerb is how a route change reads in the feed.
type ActivityVerb string

const (
	VerbCreated    ActivityVerb = "created"
	VerbEdited     ActivityVerb = "edited"
	VerbDeployed   ActivityVerb = "deployed"
	VerbPaused     ActivityVerb = "paused"
	VerbResumed    ActivityVerb = "resumed"
	VerbRolledBack ActivityVerb = "rolled_back"
)

// ActivityVerbs maps every route audit action shown in the feed to its verb.
var ActivityVerbs = map[audit.Action]ActivityVerb{
	routes.ActionRouteCreated:        VerbCreated,
	routes.ActionRouteDraftSaved:     VerbEdited,
	routes.ActionRouteUpdated:        VerbEdited,
	routes.ActionRouteDeployed:       VerbDeployed,
	routes.ActionRoutePaused:         VerbPaused,
	routes.ActionRouteResumed:        VerbResumed,
	routes.ActionRouteRolledBack:     VerbRolledBack,
	routes.ActionRouteAutoRolledBack: VerbRolledBack,
}

// ActivityDefaults are the feed's page size.
type ActivityDefaults int

const DefaultActivityLimit ActivityDefaults = 10

// ActivityItem is one feed line: "<actor> <verb> <routeName>".
type ActivityItem struct {
	ID        string       `json:"id"`
	Actor     string       `json:"actor"`
	Verb      ActivityVerb `json:"verb"`
	RouteID   string       `json:"routeId"`
	RouteName string       `json:"routeName"`
	Detail    string       `json:"detail"`
	At        time.Time    `json:"at"`
}

// ActivityPage is one page of the feed.
type ActivityPage struct {
	Items []ActivityItem
	Next  *store.Cursor
}

// AuditQuerier is the slice of access-control's audit Manager the feed uses.
type AuditQuerier interface {
	Query(ctx context.Context, f audit.Filter, page audit.Page) (audit.QueryResult, error)
}

// ActivityService reads the latest route changes from the audit log.
type ActivityService struct {
	Audit AuditQuerier
}

// List returns one page of route changes, newest first. Deploys and rollbacks are audited on the route
// version, so the actions alone select the feed (no object-type filter).
func (s ActivityService) List(ctx context.Context, limit int, after *store.Cursor) (ActivityPage, error) {
	actions := make([]audit.Action, 0, len(ActivityVerbs))
	for a := range ActivityVerbs {
		actions = append(actions, a)
	}
	res, err := s.Audit.Query(ctx, audit.Filter{Actions: actions}, audit.Page{Limit: limit, After: after})
	if err != nil {
		return ActivityPage{}, err
	}
	items := make([]ActivityItem, 0, len(res.Items))
	for _, e := range res.Items {
		routeID, ok := ActivityRoute{}.Of(e)
		if !ok {
			continue
		}
		items = append(items, ActivityItem{ID: strconv.FormatInt(e.ID, 10), Actor: e.Actor.Name, Verb: ActivityVerbs[e.Action],
			RouteID: routeID, RouteName: RouteLabel{}.Of(e), At: e.At})
	}
	return ActivityPage{Items: items, Next: res.Next}, nil
}

// ActivityRoute finds the route an audit entry is about.
type ActivityRoute struct{}

// Of returns the route id: the object id of route entries, the recorded routeId of route-version entries
// (deploy, rollback); false when a version entry predates that field.
func (ActivityRoute) Of(e audit.LogEntry) (string, bool) {
	if e.ObjectType != audit.ObjectRouteVersion {
		return e.ObjectID, true
	}
	var view struct {
		RouteID string `json:"routeId"`
	}
	if err := json.Unmarshal(e.Diff, &view); err != nil || view.RouteID == "" {
		return "", false
	}
	return view.RouteID, true
}

// RouteLabel picks the display name of an entry's route.
type RouteLabel struct{}

// Of returns the recorded label, or the route id when none was recorded.
func (RouteLabel) Of(e audit.LogEntry) string {
	if e.ObjectLabel != "" {
		return e.ObjectLabel
	}
	return e.ObjectID
}
