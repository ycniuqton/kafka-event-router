package lookups

import (
	"crypto/x509"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/pkg/lookup"
)

// FieldPath is the JSON Pointer of an input member in validation errors.
type FieldPath string

const (
	PathName           FieldPath = "/name"
	PathDescription    FieldPath = "/description"
	PathKind           FieldPath = "/kind"
	PathEnvTag         FieldPath = "/envTag"
	PathOwnerTeam      FieldPath = "/owner/teamId"
	PathHTTP           FieldPath = "/http"
	PathBaseURL        FieldPath = "/http/baseUrl"
	PathAuthMode       FieldPath = "/http/auth/mode"
	PathAuthUser       FieldPath = "/http/auth/username"
	PathAuthHeader     FieldPath = "/http/auth/headerName"
	PathCAPEM          FieldPath = "/http/tls/caPem"
	PathRedis          FieldPath = "/redis"
	PathRedisAddr      FieldPath = "/redis/addr"
	PathRedisDB        FieldPath = "/redis/db"
	PathPostgres       FieldPath = "/postgres"
	PathPgHost         FieldPath = "/postgres/host"
	PathPgPort         FieldPath = "/postgres/port"
	PathPgDatabase     FieldPath = "/postgres/database"
	PathPgUser         FieldPath = "/postgres/user"
	PathPgSSLMode      FieldPath = "/postgres/sslMode"
	PathPgTimeout      FieldPath = "/postgres/statementTimeoutMs"
	PathStatic         FieldPath = "/static"
	PathStaticTable    FieldPath = "/static/tableRef"
	PathStaticVersion  FieldPath = "/static/version"
	PathMaxConcurrency FieldPath = "/limits/maxConcurrency"
	PathRate           FieldPath = "/limits/ratePerSec"
	PathBreaker        FieldPath = "/limits/breakerFailures"
	PathBreakerOpen    FieldPath = "/limits/breakerOpenMs"
	PathSecretsPrefix  FieldPath = "/secrets/"
)

// ErrorCode is the machine code of a validation error.
type ErrorCode string

const (
	CodeRequired ErrorCode = "required"
	CodeInvalid  ErrorCode = "invalid"
	CodeRange    ErrorCode = "out_of_range"
)

// RuleMessage holds the user-facing validation texts.
type RuleMessage string

const (
	MsgSectionRequired RuleMessage = "Fill in the settings of this kind."
	MsgBaseURL         RuleMessage = "Enter an http:// or https:// URL, for example https://crm.internal/api."
	MsgAuthMode        RuleMessage = "Pick none, bearer, basic or header."
	MsgAuthUser        RuleMessage = "Enter the user name for basic auth."
	MsgAuthHeader      RuleMessage = "Enter the header name, for example X-Api-Key."
	MsgCAPEM           RuleMessage = "Paste a PEM encoded CA certificate."
	MsgSecretRequired  RuleMessage = "Enter the secret for this auth mode."
	MsgRedisAddr       RuleMessage = "host:port expected, for example redis.internal:6379."
	MsgRedisDB         RuleMessage = "Pick a database between 0 and 15."
	MsgPgHost          RuleMessage = "Enter the database host."
	MsgPgPort          RuleMessage = "Use a port between 1 and 65535."
	MsgPgDatabase      RuleMessage = "Enter the database name."
	MsgPgUser          RuleMessage = "Enter the (read-only) database user."
	MsgPgSSLMode       RuleMessage = "Pick disable, allow, prefer, require, verify-ca or verify-full."
	MsgPgTimeout       RuleMessage = "Use a statement timeout between 1 and 60000 ms."
	MsgStaticTable     RuleMessage = "Pick a library lookup table."
	MsgStaticVersion   RuleMessage = "Pin a table version (1 or higher)."
)

// KindBounds are numeric bounds of kind settings.
type KindBounds int

const (
	MaxRedisDB         KindBounds = 15
	MaxPort            KindBounds = 65535
	MaxStatementMs     KindBounds = 60000
	MaxConcurrency     KindBounds = 256
	MaxBreakerFailures KindBounds = 1000
	MaxBreakerOpenMs   KindBounds = 600000
)

// PostgresSSLModes lists the accepted libpq sslmode values.
var PostgresSSLModes = []string{"disable", "allow", "prefer", "require", "verify-ca", "verify-full"}

// URLSchemes lists the accepted HTTP source schemes.
var URLSchemes = []string{"http", "https"}

// KindSpec is the base of every source kind: its settings validation and the secrets it uses.
type KindSpec interface {
	Kind() lookup.SourceKind
	Validate(d Draft, v *api.ValidationError)
	SecretFields(d Draft) []lookup.SecretField
	Defaults(d *Draft)
}

// KindManager selects the KindSpec of a source kind (the service talks only to it).
type KindManager struct {
	kinds map[lookup.SourceKind]KindSpec
}

// NewKindManager registers kinds.
func NewKindManager(ks ...KindSpec) *KindManager {
	m := &KindManager{kinds: map[lookup.SourceKind]KindSpec{}}
	for _, k := range ks {
		m.kinds[k.Kind()] = k
	}
	return m
}

// DefaultKinds registers HTTP, Redis, Postgres and static.
func DefaultKinds() *KindManager {
	return NewKindManager(HTTPKind{}, RedisKind{}, PostgresKind{}, StaticKind{})
}

// Get returns the kind spec.
func (m *KindManager) Get(k lookup.SourceKind) (KindSpec, bool) {
	spec, ok := m.kinds[k]
	return spec, ok
}

// Violation adds one field error.
type Violation struct{}

// Add records code/message at path.
func (Violation) Add(v *api.ValidationError, path FieldPath, code ErrorCode, msg RuleMessage) {
	v.Add(string(path), string(code), string(msg))
}

// HTTPKind validates HTTP sources.
type HTTPKind struct{}

func (HTTPKind) Kind() lookup.SourceKind { return lookup.KindHTTP }

func (k HTTPKind) Validate(d Draft, v *api.ValidationError) {
	s := d.Settings.HTTP
	if s == nil {
		Violation{}.Add(v, PathHTTP, CodeRequired, MsgSectionRequired)
		return
	}
	u, err := url.Parse(s.BaseURL)
	if err != nil || u.Host == "" || !slices.Contains(URLSchemes, u.Scheme) {
		Violation{}.Add(v, PathBaseURL, CodeInvalid, MsgBaseURL)
	}
	k.validateAuth(s.Auth, v)
	k.validateSecrets(d, v)
	if s.TLS.CAPEM != "" && !x509.NewCertPool().AppendCertsFromPEM([]byte(s.TLS.CAPEM)) {
		Violation{}.Add(v, PathCAPEM, CodeInvalid, MsgCAPEM)
	}
}

func (HTTPKind) validateAuth(a lookup.HTTPAuth, v *api.ValidationError) {
	if _, ok := lookup.AuthAppliers[a.Mode]; !ok {
		Violation{}.Add(v, PathAuthMode, CodeInvalid, MsgAuthMode)
	}
	if a.Mode == lookup.AuthBasic && strings.TrimSpace(a.Username) == "" {
		Violation{}.Add(v, PathAuthUser, CodeRequired, MsgAuthUser)
	}
	if a.Mode == lookup.AuthHeader && http.CanonicalHeaderKey(strings.TrimSpace(a.HeaderName)) == "" {
		Violation{}.Add(v, PathAuthHeader, CodeRequired, MsgAuthHeader)
	}
}

func (k HTTPKind) validateSecrets(d Draft, v *api.ValidationError) {
	for _, f := range k.SecretFields(d) {
		if !d.Secrets.Has(f) {
			Violation{}.Add(v, PathSecretsPrefix+FieldPath(f), CodeRequired, MsgSecretRequired)
		}
	}
}

func (HTTPKind) SecretFields(d Draft) []lookup.SecretField {
	if d.Settings.HTTP == nil {
		return nil
	}
	if a, ok := lookup.AuthAppliers[d.Settings.HTTP.Auth.Mode]; ok {
		return a.Secrets()
	}
	return nil
}

func (HTTPKind) Defaults(d *Draft) {
	if d.Settings.HTTP != nil && d.Settings.HTTP.Auth.Mode == "" {
		d.Settings.HTTP.Auth.Mode = lookup.AuthNone
	}
}

// RedisKind validates Redis sources (password optional).
type RedisKind struct{}

func (RedisKind) Kind() lookup.SourceKind { return lookup.KindRedis }

func (RedisKind) Validate(d Draft, v *api.ValidationError) {
	s := d.Settings.Redis
	if s == nil {
		Violation{}.Add(v, PathRedis, CodeRequired, MsgSectionRequired)
		return
	}
	if !(HostPort{}).Valid(s.Addr) {
		Violation{}.Add(v, PathRedisAddr, CodeInvalid, MsgRedisAddr)
	}
	if s.DB < 0 || s.DB > int(MaxRedisDB) {
		Violation{}.Add(v, PathRedisDB, CodeRange, MsgRedisDB)
	}
}

func (RedisKind) SecretFields(Draft) []lookup.SecretField {
	return []lookup.SecretField{lookup.SecretPassword}
}

func (RedisKind) Defaults(*Draft) {}

// PostgresKind validates read-only Postgres sources.
type PostgresKind struct{}

func (PostgresKind) Kind() lookup.SourceKind { return lookup.KindPostgres }

func (PostgresKind) Validate(d Draft, v *api.ValidationError) {
	s := d.Settings.Postgres
	if s == nil {
		Violation{}.Add(v, PathPostgres, CodeRequired, MsgSectionRequired)
		return
	}
	required := []struct {
		path  FieldPath
		value string
		msg   RuleMessage
	}{{PathPgHost, s.Host, MsgPgHost}, {PathPgDatabase, s.Database, MsgPgDatabase}, {PathPgUser, s.User, MsgPgUser}}
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			Violation{}.Add(v, r.path, CodeRequired, r.msg)
		}
	}
	if s.Port < 1 || s.Port > int(MaxPort) {
		Violation{}.Add(v, PathPgPort, CodeRange, MsgPgPort)
	}
	if s.SSLMode != "" && !slices.Contains(PostgresSSLModes, s.SSLMode) {
		Violation{}.Add(v, PathPgSSLMode, CodeInvalid, MsgPgSSLMode)
	}
	if s.StatementTimeoutMs < 1 || s.StatementTimeoutMs > int(MaxStatementMs) {
		Violation{}.Add(v, PathPgTimeout, CodeRange, MsgPgTimeout)
	}
}

func (PostgresKind) SecretFields(Draft) []lookup.SecretField {
	return []lookup.SecretField{lookup.SecretPassword}
}

func (PostgresKind) Defaults(d *Draft) {
	s := d.Settings.Postgres
	if s == nil {
		return
	}
	if s.Port == 0 {
		s.Port = int(lookup.DefaultPostgresPort)
	}
	if s.StatementTimeoutMs == 0 {
		s.StatementTimeoutMs = int(lookup.DefaultStatementMs)
	}
}

// StaticKind validates static sources (a pinned library lookup table).
type StaticKind struct{}

func (StaticKind) Kind() lookup.SourceKind { return lookup.KindStatic }

func (StaticKind) Validate(d Draft, v *api.ValidationError) {
	s := d.Settings.Static
	if s == nil {
		Violation{}.Add(v, PathStatic, CodeRequired, MsgSectionRequired)
		return
	}
	if strings.TrimSpace(s.TableRef) == "" {
		Violation{}.Add(v, PathStaticTable, CodeRequired, MsgStaticTable)
	}
	if s.Version < 1 {
		Violation{}.Add(v, PathStaticVersion, CodeRange, MsgStaticVersion)
	}
}

func (StaticKind) SecretFields(Draft) []lookup.SecretField { return nil }

func (StaticKind) Defaults(*Draft) {}

// HostPort validates host:port addresses.
type HostPort struct{}

// Valid reports whether addr is host:port with a port in range.
func (HostPort) Valid(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return false
	}
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= int(MaxPort)
}
