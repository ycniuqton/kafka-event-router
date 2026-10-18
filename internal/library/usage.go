package library

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/store"
)

// UsagePinSuffix narrows a kind's usage jsonpath to the pinned version numbers of the matches.
type UsagePinSuffix string

const VersionPins UsagePinSuffix = ` .version ? (@.type() == "number")`

// JSONPathVar names the variable usage jsonpaths bind to the object name ($n).
type JSONPathVar string

const VarName JSONPathVar = "n"

// usagesSQL lists routes whose draft or active version references the object ($1 match path, $2 vars, $3 pin path).
const usagesSQL = `SELECT r.id, r.name, in_draft, in_active,
       ARRAY(SELECT DISTINCT p FROM (
               SELECT (x #>> '{}')::numeric::int AS p FROM jsonb_path_query(coalesce(r.draft_config, '{}'::jsonb), $3::jsonpath, $2) x
               UNION SELECT (x #>> '{}')::numeric::int FROM jsonb_path_query(coalesce(av.config, '{}'::jsonb), $3::jsonpath, $2) x) pins
             ORDER BY p)
  FROM routes r LEFT JOIN route_versions av ON av.id = r.active_version_id,
       LATERAL (SELECT jsonb_path_exists(coalesce(r.draft_config, '{}'::jsonb), $1::jsonpath, $2) AS in_draft,
                       jsonb_path_exists(coalesce(av.config, '{}'::jsonb), $1::jsonpath, $2) AS in_active) m
 WHERE in_draft OR in_active
 ORDER BY r.name, r.id`

// UsageRepository finds the routes referencing a library object by name.
type UsageRepository struct {
	db *store.DB
}

// NewUsageRepository builds the repository.
func NewUsageRepository(db *store.DB) *UsageRepository {
	return &UsageRepository{db: db}
}

// Usages returns the routes whose draft or active version references name.
func (r *UsageRepository) Usages(ctx context.Context, k ContentKind, name string) ([]Usage, error) {
	vars := map[string]string{string(VarName): name}
	rows, err := r.db.Q(ctx).Query(ctx, usagesSQL, k.UsagePath(), vars, k.UsagePath()+string(VersionPins))
	if err != nil {
		return nil, r.db.Translate(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Usage, error) {
		var u Usage
		var id uuid.UUID
		var versions []int32
		err := row.Scan(&id, &u.RouteName, &u.InDraft, &u.InActive, &versions)
		u.RouteID = id
		for _, v := range versions {
			u.Versions = append(u.Versions, int(v))
		}
		return u, err
	})
}
