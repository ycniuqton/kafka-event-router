package dlq

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/routes"
)

// RepoPage is the page size used when walking route repositories.
type RepoPage int

const repoPageSize RepoPage = 200

// RoutesRepoSource adapts the routes feature's repositories to RouteSource.
type RoutesRepoSource struct {
	Repos routes.Repositories
}

// Route implements RouteSource; malformed or unknown ids are ErrRouteNotFound.
func (s RoutesRepoSource) Route(ctx context.Context, id string) (RouteInfo, error) {
	if _, err := uuid.Parse(id); err != nil {
		return RouteInfo{}, ErrRouteNotFound
	}
	r, err := s.Repos.Routes.Get(ctx, routes.RouteID(id))
	if errors.Is(err, routes.ErrRouteNotFound) {
		return RouteInfo{}, ErrRouteNotFound.Wrap(err)
	}
	if err != nil {
		return RouteInfo{}, err
	}
	return RouteInfoOf{}.Route(r), nil
}

// Routes implements RouteSource (archived routes are hidden).
func (s RoutesRepoSource) Routes(ctx context.Context) ([]RouteInfo, error) {
	out := []RouteInfo{}
	page := routes.PageRequest{Limit: int(repoPageSize)}
	for {
		rows, next, err := s.Repos.Routes.ListSummaries(ctx, routes.ListQuery{}, page)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, RouteInfoOf{}.Route(row.Route))
		}
		if next == nil {
			return out, nil
		}
		page.After = next
	}
}

// Versions implements RouteSource.
func (s RoutesRepoSource) Versions(ctx context.Context, routeID string) ([]VersionConfig, error) {
	out := []VersionConfig{}
	page := routes.PageRequest{Limit: int(repoPageSize)}
	for {
		vs, next, err := s.Repos.Versions.List(ctx, routes.RouteID(routeID), page)
		if err != nil {
			return nil, err
		}
		for _, v := range vs {
			out = append(out, VersionConfig{ID: string(v.ID), No: v.No, Config: v.Config})
		}
		if next == nil {
			return out, nil
		}
		page.After = next
	}
}

// Version implements RouteSource.
func (s RoutesRepoSource) Version(ctx context.Context, versionID string) (VersionConfig, error) {
	v, err := s.Repos.Versions.GetByID(ctx, routes.VersionID(versionID))
	if err != nil {
		return VersionConfig{}, err
	}
	return VersionConfig{ID: string(v.ID), No: v.No, Config: v.Config}, nil
}

// RouteInfoOf converts routes domain values.
type RouteInfoOf struct{}

// Route converts a stored route.
func (RouteInfoOf) Route(r routes.Route) RouteInfo {
	info := RouteInfo{ID: string(r.ID), Name: r.Name, TeamID: string(r.TeamID)}
	if r.ActiveVersionID != nil {
		info.ActiveVersionID = string(*r.ActiveVersionID)
	}
	return info
}
