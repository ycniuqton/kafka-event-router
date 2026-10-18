package store

import (
	"errors"
	"strconv"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/eventrouter/router/internal/platform/problem"
)

// Error is a classified store error that the API maps to a problem document.
type Error struct {
	Kind   problem.Kind
	Detail string
	Path   string
	base   *Error
	cause  error
}

func (e *Error) Error() string             { return "store: " + e.Detail }
func (e *Error) Unwrap() error             { return e.cause }
func (e *Error) ProblemKind() problem.Kind { return e.Kind }
func (e *Error) ProblemPath() string       { return e.Path }

// Is matches derived errors (e.g. a conflict with a path) against their sentinel.
func (e *Error) Is(target error) bool {
	return e == target || (e.base != nil && e.base == target)
}

var (
	// ErrNotFound is returned when a repository lookup matches no row.
	ErrNotFound = &Error{Kind: problem.KindNotFound, Detail: "The requested resource does not exist. It may have been deleted."}
	// ErrConflict is returned on unique / foreign-key violations or invalid state transitions.
	ErrConflict = &Error{Kind: problem.KindConflict, Detail: "This change conflicts with existing data. Use a different value and retry."}
	// ErrUnavailable is returned when the database cannot be reached (503, fail closed).
	ErrUnavailable = &Error{Kind: problem.KindUnavailable, Detail: "The database is temporarily unavailable. Retry in a moment."}
	// ErrPreconditionFailed is returned when an If-Match row_version is stale.
	ErrPreconditionFailed = &Error{Kind: problem.KindPreconditionFailed, Detail: "Someone else changed this. Reload to see their version, then retry."}
)

// RowVersion is the optimistic-concurrency token stored in row_version and sent as a strong ETag.
type RowVersion int64

// ETag renders the version as a strong entity tag ("7").
func (v RowVersion) ETag() string {
	return strconv.Quote(strconv.FormatInt(int64(v), 10))
}

// PreconditionFailedError carries the current row version of a stale update.
type PreconditionFailedError struct {
	Current RowVersion
}

func (e *PreconditionFailedError) Error() string             { return ErrPreconditionFailed.Error() }
func (e *PreconditionFailedError) ProblemKind() problem.Kind { return problem.KindPreconditionFailed }
func (e *PreconditionFailedError) CurrentETag() string       { return e.Current.ETag() }
func (e *PreconditionFailedError) Is(target error) bool      { return target == ErrPreconditionFailed }

// PgErrorCode names the Postgres SQLSTATE codes the store translates.
type PgErrorCode string

const (
	PgUniqueViolation     PgErrorCode = "23505"
	PgForeignKeyViolation PgErrorCode = "23503"
)

// ConstraintMap maps constraint names to the JSON Pointer of the field they guard.
type ConstraintMap struct {
	mu    sync.RWMutex
	paths map[string]string
}

// NewConstraintMap returns the map pre-loaded with the 0001_init unique constraints.
func NewConstraintMap() *ConstraintMap {
	return &ConstraintMap{paths: map[string]string{
		"teams_name_key":                 "/name",
		"users_email_key":                "/email",
		"users_sso_subject_key":          "/ssoSubject",
		"connections_team_id_name_key":   "/name",
		"routes_team_id_name_key":        "/name",
		"transforms_team_id_name_key":    "/name",
		"lookup_tables_team_id_name_key": "/name",
	}}
}

// Register adds a constraint → JSON Pointer mapping (features call this at wiring time).
func (m *ConstraintMap) Register(constraint, path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paths[constraint] = path
}

// Translate maps pgx/Postgres errors to ErrNotFound or a conflict carrying the field path.
func (m *ConstraintMap) Translate(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var connErr *pgconn.ConnectError
	if errors.As(err, &connErr) {
		return &Error{Kind: problem.KindUnavailable, Detail: ErrUnavailable.Detail, base: ErrUnavailable, cause: err}
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	code := PgErrorCode(pgErr.Code)
	if code != PgUniqueViolation && code != PgForeignKeyViolation {
		return err
	}
	m.mu.RLock()
	path := m.paths[pgErr.ConstraintName]
	m.mu.RUnlock()
	return &Error{Kind: problem.KindConflict, Detail: ErrConflict.Detail, Path: path, base: ErrConflict}
}
