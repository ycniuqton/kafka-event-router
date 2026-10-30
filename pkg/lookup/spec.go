// Package lookup is the runtime side of lookup steps: one client per source kind (HTTP, Redis,
// Postgres read-only, static library tables) behind a circuit breaker and limiter, the Gateway that
// implements ctxhelpers.SourceGateway for workers and previews, metrics and the source tester.
package lookup

import (
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// SourceKind is the kind of a lookup source.
type SourceKind string

const (
	KindHTTP     SourceKind = "http"
	KindRedis    SourceKind = "redis"
	KindPostgres SourceKind = "postgres"
	KindStatic   SourceKind = "static"
)

// SourceKinds lists every kind.
var SourceKinds = []SourceKind{KindHTTP, KindRedis, KindPostgres, KindStatic}

// Valid reports whether k is known.
func (k SourceKind) Valid() bool {
	_, ok := RequestKindOf[k]
	return ok
}

// RequestKindOf is the lookup request kind each source kind serves.
var RequestKindOf = map[SourceKind]ctxhelpers.SourceRequestKind{
	KindHTTP:     ctxhelpers.RequestHTTP,
	KindRedis:    ctxhelpers.RequestRedis,
	KindPostgres: ctxhelpers.RequestSQL,
	KindStatic:   ctxhelpers.RequestStatic,
}

// HTTPAuthMode is how HTTP sources authenticate.
type HTTPAuthMode string

const (
	AuthNone   HTTPAuthMode = "none"
	AuthBearer HTTPAuthMode = "bearer"
	AuthBasic  HTTPAuthMode = "basic"
	AuthHeader HTTPAuthMode = "header"
)

// HTTPAuth is the non-secret part of HTTP authentication.
type HTTPAuth struct {
	Mode       HTTPAuthMode `json:"mode"`
	Username   string       `json:"username,omitempty"`
	HeaderName string       `json:"headerName,omitempty"`
}

// TLSSettings configures server verification of HTTP sources.
type TLSSettings struct {
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	CAPEM              string `json:"caPem,omitempty"`
}

// HTTPSettings configures an HTTP source.
type HTTPSettings struct {
	BaseURL string            `json:"baseUrl"`
	Auth    HTTPAuth          `json:"auth"`
	TLS     TLSSettings       `json:"tls"`
	Headers map[string]string `json:"headers,omitempty"`
}

// RedisSettings configures a Redis source.
type RedisSettings struct {
	Addr     string `json:"addr"`
	DB       int    `json:"db"`
	Username string `json:"username,omitempty"`
	TLS      bool   `json:"tls"`
}

// PostgresSettings configures a read-only Postgres source.
type PostgresSettings struct {
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Database           string `json:"database"`
	User               string `json:"user"`
	SSLMode            string `json:"sslMode"`
	StatementTimeoutMs int    `json:"statementTimeoutMs"`
}

// StaticSettings points a static source at a pinned library lookup table.
type StaticSettings struct {
	TableRef string `json:"tableRef"`
	Version  int    `json:"version"`
}

// Settings holds the kind settings of a source; exactly the member of its kind is set.
type Settings struct {
	HTTP     *HTTPSettings     `json:"http,omitempty"`
	Redis    *RedisSettings    `json:"redis,omitempty"`
	Postgres *PostgresSettings `json:"postgres,omitempty"`
	Static   *StaticSettings   `json:"static,omitempty"`
}

// Limits bound how hard one worker may call one source.
type Limits struct {
	MaxConcurrency  int     `json:"maxConcurrency"`
	RatePerSec      float64 `json:"ratePerSec"`
	Burst           int     `json:"burst"`
	BreakerFailures int     `json:"breakerFailures"`
	BreakerOpenMs   int     `json:"breakerOpenMs"`
}

// LimitDefault holds the documented limit defaults.
type LimitDefault int

const (
	DefaultMaxConcurrency  LimitDefault = 16
	DefaultBreakerFailures LimitDefault = 5
	DefaultBreakerOpenMs   LimitDefault = 10000
	DefaultStatementMs     LimitDefault = 2000
	DefaultPostgresPort    LimitDefault = 5432
)

// DefaultLimits returns the limits of a new source.
func DefaultLimits() Limits {
	return Limits{MaxConcurrency: int(DefaultMaxConcurrency), BreakerFailures: int(DefaultBreakerFailures), BreakerOpenMs: int(DefaultBreakerOpenMs)}
}

// SourceSpec is a resolved source with its decrypted secrets (internal spec endpoint → workers, previews).
type SourceSpec struct {
	ID      string       `json:"id"`
	TeamID  string       `json:"teamId,omitempty"`
	Name    string       `json:"name"`
	Kind    SourceKind   `json:"kind"`
	Version int64        `json:"version"`
	Limits  Limits       `json:"limits"`
	Secrets SecretBundle `json:"secrets"`
	Settings
}
