package pgstore

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/lookups"
	"github.com/eventrouter/router/internal/store"
)

// SQL of the usage lookup: any lookup step (also nested in if_else) naming the source, in the draft or active config.
const (
	usagePath   = `$.** ? (@.kind == "lookup" && @.lookup.sourceId == $id)`
	routesUsing = `SELECT r.id, r.name FROM routes r
  LEFT JOIN route_versions v ON v.id = r.active_version_id
 WHERE r.desired_state <> 'archived'
   AND (jsonb_path_exists(r.draft_config, $2::jsonpath, jsonb_build_object('id', $1::text))
     OR jsonb_path_exists(v.config, $2::jsonpath, jsonb_build_object('id', $1::text)))
 ORDER BY r.name, r.id`
	usageCounts = `SELECT s.id, count(r.id) FROM unnest($1::uuid[]) AS s(id)
  LEFT JOIN routes r ON r.desired_state <> 'archived' AND (
       jsonb_path_exists(r.draft_config, $2::jsonpath, jsonb_build_object('id', s.id::text))
    OR EXISTS (SELECT 1 FROM route_versions v WHERE v.id = r.active_version_id
                 AND jsonb_path_exists(v.config, $2::jsonpath, jsonb_build_object('id', s.id::text))))
 GROUP BY s.id`
)

// UsageResolver implements lookups.UsageResolver over routes.draft_config and active route_versions.config.
type UsageResolver struct {
	db *store.DB
}

// NewUsageResolver builds the resolver.
func NewUsageResolver(db *store.DB) *UsageResolver { return &UsageResolver{db: db} }

// RoutesUsing implements lookups.UsageResolver.
func (u *UsageResolver) RoutesUsing(ctx context.Context, id uuid.UUID) ([]lookups.RouteRef, error) {
	rows, err := u.db.Q(ctx).Query(ctx, routesUsing, id, usagePath)
	if err != nil {
		return nil, u.db.Translate(err)
	}
	defer rows.Close()
	out := []lookups.RouteRef{}
	for rows.Next() {
		var ref lookups.RouteRef
		if err := rows.Scan(&ref.ID, &ref.Name); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, u.db.Translate(rows.Err())
}

// CountsFor implements lookups.UsageResolver.
func (u *UsageResolver) CountsFor(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int, error) {
	out := make(map[uuid.UUID]int, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := u.db.Q(ctx).Query(ctx, usageCounts, ids, usagePath)
	if err != nil {
		return nil, u.db.Translate(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, u.db.Translate(rows.Err())
}
