package routestore

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
)

// deploymentSelect reads deployments with their version number.
const deploymentSelect = `SELECT d.id::text, d.route_id::text, d.version_id::text, v.version_no, d.previous_version_id::text,
	d.status::text, COALESCE(d.failure_reason, ''), d.requested_by::text, d.approved_by::text,
	d.requested_at, d.approved_at, d.started_at, d.finished_at
	FROM deployments d JOIN route_versions v ON v.id = d.version_id`

// terminalStatuses get finished_at when reached.
var terminalStatuses = []string{string(routes.DeployActive), string(routes.DeployFailed), string(routes.DeploySuperseded),
	string(routes.DeployRejected), string(routes.DeployCancelled)}

// DeploymentRepository is the Postgres routes.DeploymentRepository.
type DeploymentRepository struct {
	db *store.DB
}

// NewDeploymentRepository builds the repository.
func NewDeploymentRepository(db *store.DB) *DeploymentRepository {
	return &DeploymentRepository{db: db}
}

// deploymentScanner scans deploymentSelect rows.
type deploymentScanner struct {
	d           routes.Deployment
	previous    *string
	status      string
	requestedBy *string
	approvedBy  *string
}

func (s *deploymentScanner) targets() []any {
	return []any{&s.d.ID, &s.d.RouteID, &s.d.VersionID, &s.d.VersionNo, &s.previous, &s.status, &s.d.FailureReason,
		&s.requestedBy, &s.approvedBy, &s.d.RequestedAt, &s.d.ApprovedAt, &s.d.StartedAt, &s.d.FinishedAt}
}

func (s *deploymentScanner) deployment() routes.Deployment {
	vals := Values{}
	s.d.PreviousVersionID, s.d.Status = vals.VersionID(s.previous), routes.DeploymentStatus(s.status)
	s.d.RequestedBy, s.d.ApprovedBy = vals.UserID(s.requestedBy), vals.UserID(s.approvedBy)
	return s.d
}

// Create inserts a deployment; rolling_out deployments start now.
func (repo *DeploymentRepository) Create(ctx context.Context, d routes.Deployment) (routes.Deployment, error) {
	var id string
	err := repo.db.Q(ctx).QueryRow(ctx, `INSERT INTO deployments (route_id, version_id, previous_version_id, status, requested_by, started_at)
		VALUES ($1::text::uuid, $2::text::uuid, $3::text::uuid, $4::text::deployment_status, $5,
		CASE WHEN $4::text = $6::text THEN now() END)
		RETURNING id::text`, string(d.RouteID), string(d.VersionID), d.PreviousVersionID, string(d.Status), d.RequestedBy,
		string(routes.DeployRollingOut)).Scan(&id)
	if err != nil {
		return routes.Deployment{}, repo.db.Translate(err)
	}
	return repo.Get(ctx, routes.DeploymentID(id))
}

// Get reads one deployment.
func (repo *DeploymentRepository) Get(ctx context.Context, id routes.DeploymentID) (routes.Deployment, error) {
	did, err := Params{}.UUID(string(id))
	if err != nil {
		return routes.Deployment{}, err
	}
	return repo.one(ctx, deploymentSelect+` WHERE d.id = $1`, did)
}

// SupersedeOpen marks the route's deployments in the scope's statuses superseded (scope.Except is kept).
func (repo *DeploymentRepository) SupersedeOpen(ctx context.Context, id routes.RouteID, scope routes.SupersedeScope) error {
	rid, err := Params{}.UUID(string(id))
	if err != nil {
		return err
	}
	statuses := make([]string, 0, len(scope.Statuses))
	for _, s := range scope.Statuses {
		statuses = append(statuses, string(s))
	}
	_, err = repo.db.Q(ctx).Exec(ctx, `UPDATE deployments SET status = $1::text::deployment_status, finished_at = now()
		WHERE route_id = $2 AND status::text = ANY($3::text[]) AND id::text <> $4`,
		string(routes.DeploySuperseded), rid, statuses, string(scope.Except))
	return repo.db.Translate(err)
}

// Transition moves a deployment from → to; a deployment in another status yields ErrTransitionNotAllowed.
func (repo *DeploymentRepository) Transition(ctx context.Context, c routes.StatusChange) (routes.Deployment, error) {
	did, err := Params{}.UUID(string(c.ID))
	if err != nil {
		return routes.Deployment{}, err
	}
	var id string
	err = repo.db.Q(ctx).QueryRow(ctx, `UPDATE deployments SET status = $1::text::deployment_status,
		failure_reason = COALESCE(NULLIF($2::text, ''), failure_reason),
		approved_by = COALESCE($3::uuid, approved_by),
		approved_at = CASE WHEN $3::uuid IS NOT NULL THEN now() ELSE approved_at END,
		started_at = CASE WHEN $1::text = $6::text THEN now() ELSE started_at END,
		finished_at = CASE WHEN $1::text = ANY($7::text[]) THEN now() ELSE finished_at END
		WHERE id = $4 AND status::text = $5::text RETURNING id::text`,
		string(c.To), c.Reason, c.ApprovedBy, did, string(c.From), string(routes.DeployRollingOut), terminalStatuses).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return routes.Deployment{}, repo.missingOrMoved(ctx, c.ID)
	}
	if err != nil {
		return routes.Deployment{}, repo.db.Translate(err)
	}
	return repo.Get(ctx, c.ID)
}

// missingOrMoved distinguishes an unknown deployment from one in another status.
func (repo *DeploymentRepository) missingOrMoved(ctx context.Context, id routes.DeploymentID) error {
	if _, err := repo.Get(ctx, id); err != nil {
		return err
	}
	return routes.ErrTransitionNotAllowed
}

// Latest returns the newest deployment of a route, or nil.
func (repo *DeploymentRepository) Latest(ctx context.Context, id routes.RouteID) (*routes.Deployment, error) {
	rid, err := Params{}.UUID(string(id))
	if err != nil {
		return nil, err
	}
	d, err := repo.one(ctx, deploymentSelect+` WHERE d.route_id = $1 ORDER BY d.requested_at DESC, d.id DESC LIMIT 1`, rid)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// List returns deployments newest first, keyset-paged by requested_at.
func (repo *DeploymentRepository) List(ctx context.Context, id routes.RouteID, page routes.PageRequest) ([]routes.Deployment, *store.Cursor, error) {
	rid, err := Params{}.UUID(string(id))
	if err != nil {
		return nil, nil, err
	}
	sql, args := store.KeysetQuery{Select: deploymentSelect, Where: []string{"d.route_id = $1"}, Args: []any{rid},
		SortColumn: "d.requested_at", SortCast: "timestamptz", IDColumn: "d.id", IDCast: "uuid", Limit: page.Limit, After: page.After}.Build()
	rows, err := repo.db.Q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, repo.db.Translate(err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (routes.Deployment, error) {
		var s deploymentScanner
		err := row.Scan(s.targets()...)
		return s.deployment(), err
	})
	if err != nil {
		return nil, nil, repo.db.Translate(err)
	}
	kept, next := store.PageSlicer[routes.Deployment]{Limit: page.Limit, KeyOf: func(d routes.Deployment) store.Cursor {
		return store.Cursor{SortKey: []string{CursorTime{}.Key(d.RequestedAt)}, ID: string(d.ID)}
	}}.Slice(list)
	return kept, next, nil
}

// StatusByVersion returns the latest deployment status of each version.
func (repo *DeploymentRepository) StatusByVersion(ctx context.Context, ids []routes.VersionID) (map[routes.VersionID]routes.DeploymentStatus, error) {
	out := map[routes.VersionID]routes.DeploymentStatus{}
	if len(ids) == 0 {
		return out, nil
	}
	texts := make([]string, len(ids))
	for i, id := range ids {
		texts[i] = string(id)
	}
	rows, err := repo.db.Q(ctx).Query(ctx, `SELECT DISTINCT ON (version_id) version_id::text, status::text FROM deployments
		WHERE version_id::text = ANY($1::text[]) ORDER BY version_id, requested_at DESC, id DESC`, texts)
	if err != nil {
		return nil, repo.db.Translate(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, repo.db.Translate(err)
		}
		out[routes.VersionID(id)] = routes.DeploymentStatus(status)
	}
	return out, repo.db.Translate(rows.Err())
}

func (repo *DeploymentRepository) one(ctx context.Context, sql string, args ...any) (routes.Deployment, error) {
	var s deploymentScanner
	if err := repo.db.Q(ctx).QueryRow(ctx, sql, args...).Scan(s.targets()...); err != nil {
		return routes.Deployment{}, repo.db.Translate(err)
	}
	return s.deployment(), nil
}
