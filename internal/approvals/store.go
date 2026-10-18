package approvals

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
)

// PostgresStore implements Store on deployments, route_versions, routes, users and approval_comments.
type PostgresStore struct {
	db *store.DB
}

// NewPostgresStore builds the store.
func NewPostgresStore(db *store.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// selectApprovalsSQL reads approvals; only deployments that waited for a decision are approvals.
const selectApprovalsSQL = `SELECT d.id, d.route_id, r.name, r.team_id, d.version_id, v.version_no, d.status::text,
	d.requested_by, COALESCE(ru.display_name, ''), d.requested_at, COALESCE(v.change_note, ''),
	COALESCE(d.approved_by, d.rejected_by), COALESCE(du.display_name, ''), COALESCE(d.approved_at, d.rejected_at),
	COALESCE(d.decision_note, ''), COALESCE(d.cancel_reason, ''), d.row_version, v.config, d.finished_at
	FROM deployments d JOIN routes r ON r.id = d.route_id JOIN route_versions v ON v.id = d.version_id
	LEFT JOIN users ru ON ru.id = d.requested_by LEFT JOIN users du ON du.id = COALESCE(d.approved_by, d.rejected_by)`

// approvalScope keeps deployments that were requested for approval (never started without a decision).
const approvalScope = `(d.started_at IS NULL OR d.approved_by IS NOT NULL)`

// ApprovalSort names the keyset columns of approval lists (newest request first).
type ApprovalSort string

const (
	SortColumn ApprovalSort = "d.requested_at"
	SortCast   ApprovalSort = "timestamptz"
	IDColumn   ApprovalSort = "d.id"
	IDCast     ApprovalSort = "uuid"
)

// List returns one page of approvals matching f.
func (s *PostgresStore) List(ctx context.Context, f ListFilter, page Page) ([]StoredApproval, *store.Cursor, error) {
	where, args := ListWhere{}.Build(f)
	sql, args := store.KeysetQuery{Select: selectApprovalsSQL, Where: append([]string{approvalScope}, where...), Args: args,
		SortColumn: string(SortColumn), SortCast: string(SortCast), IDColumn: string(IDColumn), IDCast: string(IDCast),
		Direction: store.SortDesc, Limit: page.Limit, After: page.After}.Build()
	items, err := s.query(ctx, sql, args...)
	if err != nil {
		return nil, nil, err
	}
	out, next := store.PageSlicer[StoredApproval]{Limit: page.Limit, KeyOf: func(a StoredApproval) store.Cursor {
		return store.Cursor{SortKey: []string{a.RequestedAt.Format(time.RFC3339Nano)}, ID: a.ID.String()}
	}}.Slice(items)
	return out, next, nil
}

// Get returns one approval (store.ErrNotFound when the deployment is not an approval).
func (s *PostgresStore) Get(ctx context.Context, id uuid.UUID) (StoredApproval, error) {
	return s.one(ctx, selectApprovalsSQL+` WHERE d.id = $1 AND `+approvalScope, id)
}

// Lock returns one approval with its deployment row locked for the decision.
func (s *PostgresStore) Lock(ctx context.Context, id uuid.UUID) (StoredApproval, error) {
	return s.one(ctx, selectApprovalsSQL+` WHERE d.id = $1 AND `+approvalScope+` FOR UPDATE OF d`, id)
}

// ActiveVersion returns the route's live version, or nil.
func (s *PostgresStore) ActiveVersion(ctx context.Context, route uuid.UUID) (*VersionConfig, error) {
	var v VersionConfig
	err := s.db.Q(ctx).QueryRow(ctx, `SELECT v.version_no, v.config FROM routes r JOIN route_versions v ON v.id = r.active_version_id
		WHERE r.id = $1`, route).Scan(&v.No, &v.Config)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, s.db.Translate(err)
	}
	return &v, nil
}

// SetDecisionNote stores the approver's note before activation.
func (s *PostgresStore) SetDecisionNote(ctx context.Context, id uuid.UUID, note string) error {
	_, err := s.db.Q(ctx).Exec(ctx, `UPDATE deployments SET decision_note = NULLIF($2, '') WHERE id = $1`, id, note)
	return s.db.Translate(err)
}

// Reject moves a pending request to rejected (its version number stays consumed).
func (s *PostgresStore) Reject(ctx context.Context, id, by uuid.UUID, note string) error {
	return s.decide(ctx, `UPDATE deployments SET status = $2::text::deployment_status, rejected_by = $3, rejected_at = now(),
		decision_note = $4, finished_at = now() WHERE id = $1 AND status::text = $5`, id, string(StatusRejected), by, note, string(StatusPending))
}

// Cancel moves a pending request to cancelled with a reason.
func (s *PostgresStore) Cancel(ctx context.Context, id uuid.UUID, reason CancelReason) error {
	return s.decide(ctx, `UPDATE deployments SET status = $2::text::deployment_status, cancel_reason = $3, finished_at = now()
		WHERE id = $1 AND status::text = $4`, id, string(StatusCancelled), string(reason), string(StatusPending))
}

// CancelOtherPending cancels the route's older pending requests (superseded by a newer one).
func (s *PostgresStore) CancelOtherPending(ctx context.Context, route, except uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.db.Q(ctx).Query(ctx, `UPDATE deployments SET status = $3::text::deployment_status, cancel_reason = $4, finished_at = now()
		WHERE route_id = $1 AND id <> $2 AND status::text = $5 RETURNING id`, route, except, string(StatusCancelled),
		string(CancelSuperseded), string(StatusPending))
	if err != nil {
		return nil, s.db.Translate(err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// Comments returns the request's comments, oldest first.
func (s *PostgresStore) Comments(ctx context.Context, id uuid.UUID) ([]Comment, error) {
	rows, err := s.db.Q(ctx).Query(ctx, `SELECT c.id, c.deployment_id, c.author_id, COALESCE(u.display_name, ''), c.body, c.created_at
		FROM approval_comments c LEFT JOIN users u ON u.id = c.author_id WHERE c.deployment_id = $1 ORDER BY c.created_at, c.id`, id)
	if err != nil {
		return nil, s.db.Translate(err)
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		c, err := s.scanComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddComment inserts a comment.
func (s *PostgresStore) AddComment(ctx context.Context, id, author uuid.UUID, body string) (Comment, error) {
	row := s.db.Q(ctx).QueryRow(ctx, `WITH c AS (INSERT INTO approval_comments (deployment_id, author_id, body) VALUES ($1, $2, $3)
		RETURNING id, deployment_id, author_id, body, created_at)
		SELECT c.id, c.deployment_id, c.author_id, COALESCE(u.display_name, ''), c.body, c.created_at FROM c LEFT JOIN users u ON u.id = c.author_id`,
		id, author, body)
	c, err := s.scanComment(row)
	return c, s.db.Translate(err)
}

// Approvers returns active users who may approve in team (approver/admin binding or workspace admin), excluding one.
func (s *PostgresStore) Approvers(ctx context.Context, team rbac.TeamID, excluding uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.db.Q(ctx).Query(ctx, `SELECT u.id FROM users u WHERE u.disabled_at IS NULL AND u.sso_subject IS NOT NULL AND u.id <> $2
		AND (u.is_workspace_admin OR EXISTS (SELECT 1 FROM role_bindings rb WHERE rb.user_id = u.id AND rb.team_id = $1
		AND rb.role::text = ANY($3))) ORDER BY u.id`, team, excluding, ApproverRoles{}.Names())
	if err != nil {
		return nil, s.db.Translate(err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// Commenters returns the distinct authors of the request's comments.
func (s *PostgresStore) Commenters(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.db.Q(ctx).Query(ctx, `SELECT DISTINCT author_id FROM approval_comments WHERE deployment_id = $1 AND author_id IS NOT NULL`, id)
	if err != nil {
		return nil, s.db.Translate(err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// PendingByTeam counts pending requests per team (metrics gauge).
func (s *PostgresStore) PendingByTeam(ctx context.Context) (map[rbac.TeamID]int, error) {
	rows, err := s.db.Q(ctx).Query(ctx, `SELECT r.team_id, count(*) FROM deployments d JOIN routes r ON r.id = d.route_id
		WHERE d.status::text = $1 GROUP BY r.team_id`, string(StatusPending))
	if err != nil {
		return nil, s.db.Translate(err)
	}
	defer rows.Close()
	out := map[rbac.TeamID]int{}
	for rows.Next() {
		var team rbac.TeamID
		var n int
		if err := rows.Scan(&team, &n); err != nil {
			return nil, err
		}
		out[team] = n
	}
	return out, rows.Err()
}

// TeamOf implements rbac.TeamOwner for approval ids (the owning route's team).
func (s *PostgresStore) TeamOf(ctx context.Context, id string) (string, error) {
	var team string
	err := s.db.Q(ctx).QueryRow(ctx, `SELECT r.team_id::text FROM deployments d JOIN routes r ON r.id = d.route_id WHERE d.id::text = $1`, id).Scan(&team)
	return team, s.db.Translate(err)
}

func (s *PostgresStore) decide(ctx context.Context, sql string, args ...any) error {
	tag, err := s.db.Q(ctx).Exec(ctx, sql, args...)
	if err != nil {
		return s.db.Translate(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotPending
	}
	return nil
}

func (s *PostgresStore) one(ctx context.Context, sql string, args ...any) (StoredApproval, error) {
	items, err := s.query(ctx, sql, args...)
	if err != nil {
		return StoredApproval{}, err
	}
	if len(items) == 0 {
		return StoredApproval{}, store.ErrNotFound
	}
	return items[0], nil
}

func (s *PostgresStore) query(ctx context.Context, sql string, args ...any) ([]StoredApproval, error) {
	rows, err := s.db.Q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, s.db.Translate(err)
	}
	defer rows.Close()
	out := []StoredApproval{}
	for rows.Next() {
		a, err := ApprovalScanner{}.Scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *PostgresStore) scanComment(row pgx.Row) (Comment, error) {
	var c Comment
	var author *uuid.UUID
	var name string
	err := row.Scan(&c.ID, &c.ApprovalID, &author, &name, &c.Body, &c.CreatedAt)
	c.Author = PersonOf{}.Build(author, name)
	return c, err
}

// ApprovalScanner scans selectApprovalsSQL rows.
type ApprovalScanner struct{}

// Scan reads one row.
func (ApprovalScanner) Scan(row pgx.Row) (StoredApproval, error) {
	var a StoredApproval
	var status, cancel, requesterName, deciderName string
	var requester, decider *uuid.UUID
	var version int64
	var finished *time.Time
	err := row.Scan(&a.ID, &a.RouteID, &a.RouteName, &a.TeamID, &a.VersionID, &a.VersionNo, &status, &requester, &requesterName,
		&a.RequestedAt, &a.ChangeNote, &decider, &deciderName, &a.DecidedAt, &a.DecisionNote, &cancel, &version, &a.Config, &finished)
	if err != nil {
		return StoredApproval{}, err
	}
	a.Status, a.CancelReason, a.RowVersion = ApprovalStatus(status), CancelReason(cancel), store.RowVersion(version)
	a.RequestedBy, a.DecidedBy, a.Destinations = PersonOf{}.Build(requester, requesterName), PersonOf{}.Build(decider, deciderName), []Destination{}
	if a.DecidedAt == nil && a.Status == StatusCancelled {
		a.DecidedAt = finished
	}
	return a, nil
}

// PersonOf builds optional people from nullable ids.
type PersonOf struct{}

// Build returns nil for a missing id.
func (PersonOf) Build(id *uuid.UUID, name string) *Person {
	if id == nil {
		return nil
	}
	return &Person{ID: *id, Name: name}
}

// ApproverRoles are the team roles holding deployment.approve.
type ApproverRoles struct{}

// Names returns the role names of approvers (from the role matrix).
func (ApproverRoles) Names() []string {
	out := []string{}
	matrix := rbac.DefaultRoleMatrix()
	for _, r := range (rbac.Roles{}).Ordered() {
		if matrix.Permissions(r).Has(rbac.PermDeploymentApprove) {
			out = append(out, string(r))
		}
	}
	return out
}
