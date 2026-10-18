package routestore

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
)

// liveRoute is the predicate shared by every non-archived status.
const liveRoute = `r.desired_state <> 'archived' AND r.active_version_id IS NOT NULL AND rr.observed_state IS DISTINCT FROM 'error'`

// statusPredicates expresses each derived status (RouteStatusResolver) in SQL.
var statusPredicates = map[routes.Status]string{
	routes.StatusArchived: `r.desired_state = 'archived'`,
	routes.StatusDraft:    `(r.desired_state <> 'archived' AND r.active_version_id IS NULL)`,
	routes.StatusError:    `(r.desired_state <> 'archived' AND r.active_version_id IS NOT NULL AND rr.observed_state = 'error')`,
	routes.StatusPaused:   `(` + liveRoute + ` AND r.desired_state = 'paused')`,
	routes.StatusRunning:  `(` + liveRoute + ` AND r.desired_state = 'running')`,
}

// hideArchived is the default list predicate.
const hideArchived = `r.desired_state <> 'archived'`

// usesConnection matches routes whose shown config reads from or writes to a connection.
const usesConnection = `(jsonb_path_exists(COALESCE(av.config, r.draft_config), '$.source.connectionId ? (@ == $c)', jsonb_build_object('c', %[1]s::text))
	OR jsonb_path_exists(COALESCE(av.config, r.draft_config), '$.branches[*].destination.connectionId ? (@ == $c)', jsonb_build_object('c', %[1]s::text)))`

// LikeEscaper escapes LIKE wildcards in user search text.
var LikeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// summaryScanner scans a summary row.
type summaryScanner struct {
	route      rowScanner
	teamName   string
	observed   string
	activeID   *string
	activeNo   *int
	activeCfg  []byte
	activeHash *string
	userID     *string
	userName   *string
}

func (s *summaryScanner) targets() []any {
	return append(s.route.targets(), &s.teamName, &s.observed, &s.activeID, &s.activeNo, &s.activeCfg, &s.activeHash, &s.userID, &s.userName)
}

func (s *summaryScanner) row() routes.SummaryRow {
	out := routes.SummaryRow{Route: s.route.route(), TeamName: s.teamName, ObservedState: routes.ObservedState(s.observed),
		UpdatedBy: Values{}.UserRef(s.userID, s.userName)}
	if s.activeID != nil && s.activeNo != nil {
		out.Active = &routes.ActiveVersionRef{ID: routes.VersionID(*s.activeID), No: *s.activeNo, Config: Values{}.Raw(s.activeCfg)}
		if s.activeHash != nil {
			out.Active.ConfigHash = *s.activeHash
		}
	}
	return out
}

// GetSummary reads one route with its joins.
func (repo *RouteRepository) GetSummary(ctx context.Context, id routes.RouteID) (routes.SummaryRow, error) {
	uid, err := Params{}.UUID(string(id))
	if err != nil {
		return routes.SummaryRow{}, err
	}
	var s summaryScanner
	if err := repo.db.Q(ctx).QueryRow(ctx, summarySelect+` WHERE r.id = $1`, uid).Scan(s.targets()...); err != nil {
		return routes.SummaryRow{}, repo.db.Translate(err)
	}
	return s.row(), nil
}

// ListSummaries returns one keyset page ordered by (updated_at, id) descending.
func (repo *RouteRepository) ListSummaries(ctx context.Context, q routes.ListQuery, page routes.PageRequest) ([]routes.SummaryRow, *store.Cursor, error) {
	where, args := ListFilter{}.Build(q)
	sql, args := store.KeysetQuery{Select: summarySelect, Where: where, Args: args, SortColumn: "r.updated_at", SortCast: "timestamptz",
		IDColumn: "r.id", IDCast: "uuid", Limit: page.Limit, After: page.After}.Build()
	rows, err := repo.db.Q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, repo.db.Translate(err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (routes.SummaryRow, error) {
		var s summaryScanner
		err := row.Scan(s.targets()...)
		return s.row(), err
	})
	if err != nil {
		return nil, nil, repo.db.Translate(err)
	}
	kept, next := store.PageSlicer[routes.SummaryRow]{Limit: page.Limit, KeyOf: func(r routes.SummaryRow) store.Cursor {
		return store.Cursor{SortKey: []string{CursorTime{}.Key(r.Route.UpdatedAt)}, ID: string(r.Route.ID)}
	}}.Slice(list)
	return kept, next, nil
}

// ListFilter renders ListQuery as WHERE clauses with positional args.
type ListFilter struct{}

// Build returns the clauses and args for q.
func (ListFilter) Build(q routes.ListQuery) ([]string, []any) {
	where, args := []string{}, []any{}
	next := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	where = append(where, ListFilter{}.status(q.Statuses))
	if s := strings.TrimSpace(q.Search); s != "" {
		where = append(where, `r.name ILIKE '%' || `+next(LikeEscaper.Replace(s))+`::text || '%'`)
	}
	if q.Tag != "" {
		where = append(where, next(q.Tag)+`::text = ANY(r.tags)`)
	}
	if q.ConnectionID != "" {
		where = append(where, fmt.Sprintf(usesConnection, next(q.ConnectionID)))
	}
	if len(q.TeamIDs) > 0 {
		where = append(where, `r.team_id::text = ANY(`+next(TeamTexts{}.Of(q.TeamIDs))+`::text[])`)
	}
	return where, args
}

// TeamTexts renders team ids as SQL text array members.
type TeamTexts struct{}

// Of returns the ids as strings.
func (TeamTexts) Of(ids []routes.TeamID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

// status ORs the predicates of the requested statuses; none hides archived routes.
func (ListFilter) status(statuses []routes.Status) string {
	parts := []string{}
	for _, s := range statuses {
		if p, ok := statusPredicates[s]; ok {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return hideArchived
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}
