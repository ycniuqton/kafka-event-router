package lookup

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// PostgresParam names the session parameters every lookup connection runs with.
type PostgresParam string

const (
	ParamReadOnly         PostgresParam = "default_transaction_read_only"
	ParamStatementTimeout PostgresParam = "statement_timeout"
	ParamApplicationName  PostgresParam = "application_name"
	ParamOn               PostgresParam = "on"
	ApplicationName       PostgresParam = "event-router-lookup"
	SetLocalTimeout       PostgresParam = "SET LOCAL statement_timeout = "
	SampleQuery           PostgresParam = "SELECT 1"
	URLScheme             PostgresParam = "postgres"
	QuerySSLMode          PostgresParam = "sslmode"
)

// PgTransientClasses are SQLSTATE classes that heal on retry: connection, resources, operator intervention, rollback.
var PgTransientClasses = []string{"08", "53", "57", "40"}

// PgAuthClass is the SQLSTATE class of invalid authorization.
const PgAuthClass = "28"

// PgRunner executes read-only statements (pgx in production, fakes in tests).
type PgRunner interface {
	QueryRow(ctx context.Context, sql string, args []any) (map[string]any, bool, error)
	Ping(ctx context.Context) error
	Sample(ctx context.Context) error
	Close()
}

// PostgresFactory builds Postgres clients over a pgx pool.
type PostgresFactory struct{}

func (PostgresFactory) Kind() SourceKind { return KindPostgres }

func (PostgresFactory) New(spec SourceSpec) (Client, error) {
	cfg, err := PostgresConfigBuilder{}.Build(spec)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	return NewPostgresClient(&PgxRunner{pool: pool, timeoutMs: PostgresTimeout{}.Of(spec.Postgres)}), nil
}

// PostgresTimeout returns the statement timeout of a source (default 2000 ms).
type PostgresTimeout struct{}

// Of returns the timeout in ms.
func (PostgresTimeout) Of(s *PostgresSettings) int {
	if s == nil || s.StatementTimeoutMs <= 0 {
		return int(DefaultStatementMs)
	}
	return s.StatementTimeoutMs
}

// PostgresConfigBuilder builds the pool config: read-only sessions, statement timeout, pool size = max concurrency.
type PostgresConfigBuilder struct{}

// Build returns the pool config of spec.
func (PostgresConfigBuilder) Build(spec SourceSpec) (*pgxpool.Config, error) {
	s := spec.Postgres
	if s == nil {
		return nil, fmt.Errorf("postgres settings missing")
	}
	port := s.Port
	if port == 0 {
		port = int(DefaultPostgresPort)
	}
	u := url.URL{Scheme: string(URLScheme), User: url.UserPassword(s.User, spec.Secrets.Password),
		Host: net.JoinHostPort(s.Host, strconv.Itoa(port)), Path: "/" + s.Database}
	if s.SSLMode != "" {
		u.RawQuery = url.Values{string(QuerySSLMode): {s.SSLMode}}.Encode()
	}
	cfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		return nil, err
	}
	params := cfg.ConnConfig.RuntimeParams
	params[string(ParamReadOnly)] = string(ParamOn)
	params[string(ParamStatementTimeout)] = strconv.Itoa(PostgresTimeout{}.Of(s))
	params[string(ParamApplicationName)] = string(ApplicationName)
	if spec.Limits.MaxConcurrency > 0 {
		cfg.MaxConns = int32(spec.Limits.MaxConcurrency)
	}
	return cfg, nil
}

// PgxRunner runs every statement in a READ ONLY transaction with SET LOCAL statement_timeout.
type PgxRunner struct {
	pool      *pgxpool.Pool
	timeoutMs int
}

// QueryRow returns the first row as column → value.
func (r *PgxRunner) QueryRow(ctx context.Context, sql string, args []any) (map[string]any, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, string(SetLocalTimeout)+strconv.Itoa(r.timeoutMs)); err != nil {
		return nil, false, err
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, false, rows.Err()
	}
	values, err := rows.Values()
	if err != nil {
		return nil, false, err
	}
	return PgRow{}.Map(rows.FieldDescriptions(), values), true, nil
}

// Ping connects and authenticates.
func (r *PgxRunner) Ping(ctx context.Context) error { return r.pool.Ping(ctx) }

// Sample runs the read-only test query.
func (r *PgxRunner) Sample(ctx context.Context) error {
	_, _, err := r.QueryRow(ctx, string(SampleQuery), nil)
	return err
}

// Close releases the pool.
func (r *PgxRunner) Close() { r.pool.Close() }

// PgRow converts a row into engine values (integers → int64, times → RFC 3339, numerics → float64).
type PgRow struct{}

// Map pairs column names with converted values.
func (p PgRow) Map(fields []pgconn.FieldDescription, values []any) map[string]any {
	out := make(map[string]any, len(fields))
	for i, f := range fields {
		out[f.Name] = p.Value(values[i])
	}
	return out
}

// Value converts one driver value.
func (PgRow) Value(v any) any {
	switch t := v.(type) {
	case int16:
		return int64(t)
	case int32:
		return int64(t)
	case int:
		return int64(t)
	case float32:
		return float64(t)
	case time.Time:
		return t.UTC().Format(time.RFC3339Nano)
	case []byte:
		return string(t)
	case [16]byte:
		return uuid.UUID(t).String()
	case pgtype.Numeric:
		f, _ := t.Float64Value()
		return f.Float64
	}
	return v
}

// PostgresClient serves `sql` requests: first row found, no rows = miss.
type PostgresClient struct {
	runner PgRunner
}

// NewPostgresClient wraps a runner.
func NewPostgresClient(r PgRunner) *PostgresClient { return &PostgresClient{runner: r} }

// Fetch implements Client.
func (c *PostgresClient) Fetch(ctx context.Context, req ctxhelpers.SourceRequest) (ctxhelpers.SourceAnswer, error) {
	row, found, err := c.runner.QueryRow(ctx, req.SQL, req.Params)
	if err != nil {
		return ctxhelpers.SourceAnswer{}, PgErrors{}.Classify(err)
	}
	if !found {
		return ctxhelpers.SourceAnswer{Mode: ctxhelpers.AnswerLive}, nil
	}
	return ctxhelpers.SourceAnswer{Found: true, Value: row, Mode: ctxhelpers.AnswerLive}, nil
}

// Probe connects (reachable / authenticated) and runs the read-only sample query.
func (c *PostgresClient) Probe(ctx context.Context) error {
	if err := c.runner.Ping(ctx); err != nil {
		if (PgErrors{}).class(err) == PgAuthClass {
			return &ProbeError{Stage: StageAuthenticated, Hint: string(HintCredentials), Cause: err}
		}
		return &ProbeError{Stage: StageReachable, Hint: string(HintReachable), Cause: err}
	}
	if err := c.runner.Sample(ctx); err != nil {
		return &ProbeError{Stage: StageSample, Hint: string(HintReadOnly), Cause: err}
	}
	return nil
}

// Close implements Client.
func (c *PostgresClient) Close() { c.runner.Close() }

// PgErrors classifies Postgres failures by SQLSTATE class.
type PgErrors struct{}

// Classify returns transient for connection/resource/timeout classes, network errors and deadlines.
func (e PgErrors) Classify(err error) error {
	class := e.class(err)
	var ce *pgconn.ConnectError
	if class == "" && errors.As(err, &ce) {
		return Failures{}.Transient(ReasonUnavailable, err)
	}
	if class == "" {
		return Failures{}.Network(err)
	}
	for _, c := range PgTransientClasses {
		if class == c {
			return Failures{}.Transient(ReasonUnavailable, err)
		}
	}
	return Failures{}.Permanent(ReasonRejected, err)
}

func (PgErrors) class(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && len(pe.Code) >= 2 {
		return strings.ToUpper(pe.Code[:2])
	}
	var ce *pgconn.ConnectError
	if errors.As(err, &ce) {
		return PgErrors{}.class(ce.Unwrap())
	}
	return ""
}
