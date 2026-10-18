package routes

import (
	"context"
	"errors"
	"log/slog"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/routes/validation"
	"github.com/eventrouter/router/internal/store"
)

// Validator is the validation entry point managers use (validation.ValidationManager).
type Validator interface {
	Run(ctx context.Context, req validation.Request) validation.Result
}

// Core is what every routes manager shares: repositories, transactions, validation,
// views, audit, events and the write guard.
type Core struct {
	Repos     Repositories
	Tx        store.TxManager
	Validator Validator
	Views     SummaryBuilder
	Auditor   RouteAuditor
	Events    RouteEventPublisher
	Guard     WriteGuard
	Log       *slog.Logger
}

// loadWritable loads a route and checks the caller may change it.
func (c *Core) loadWritable(ctx context.Context, id RouteID) (Route, Actor, error) {
	r, err := c.load(ctx, id)
	if err != nil {
		return Route{}, Actor{}, err
	}
	actor := ActorResolver{}.From(ctx)
	if !c.Guard.CanWrite(actor, r.TeamID) {
		return Route{}, Actor{}, ErrForbiddenTeam
	}
	return r, actor, nil
}

// load reads a route, mapping a missing row to ErrRouteNotFound.
func (c *Core) load(ctx context.Context, id RouteID) (Route, error) {
	r, err := c.Repos.Routes.Get(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return Route{}, ErrRouteNotFound
	}
	return r, err
}

// detail reads the full detail view of a route.
func (c *Core) detail(ctx context.Context, id RouteID) (RouteDetail, error) {
	row, err := c.Repos.Routes.GetSummary(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return RouteDetail{}, ErrRouteNotFound
	}
	if err != nil {
		return RouteDetail{}, err
	}
	last, err := c.Repos.Deployments.Latest(ctx, id)
	if err != nil {
		return RouteDetail{}, err
	}
	return c.Views.Detail(ctx, row, last), nil
}

// publish emits a route change after commit; failures are logged, the reconciler catches up.
func (c *Core) publish(ctx context.Context, ev RouteChanged) {
	if c.Events == nil {
		return
	}
	if err := c.Events.Publish(ctx, ev); err != nil {
		c.Log.Warn("routes: publish route_changed failed", string(logging.AttrError), err)
	}
}

// draftIssues validates a route's draft in draft mode (never blocks).
func (c *Core) draftIssues(ctx context.Context, r Route) validation.Result {
	if r.Draft == nil {
		return validation.Result{Valid: true}
	}
	return c.Validator.Run(ctx, validation.Request{Mode: validation.ModeDraft, TeamID: string(r.TeamID), RouteID: string(r.ID), RouteName: r.Name, Raw: r.Draft})
}
