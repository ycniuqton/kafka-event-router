package config

import (
	"log/slog"
	"time"
)

// Config is the complete runtime configuration of one service process.
type Config struct {
	Service      ServiceName
	Environment  Environment
	Workspace    string
	HTTP         HTTPConfig
	Internal     InternalConfig
	TLS          TLSConfig
	Log          LogConfig
	Database     DatabaseConfig
	Redis        RedisConfig
	Kafka        KafkaConfig
	Secrets      SecretsConfig
	Auth         AuthConfig
	Access       AccessConfig
	ControlPlane ControlPlaneConfig
	RateLimit    RateLimitConfig
	Tracing      TracingConfig
	WebSocket    WebSocketConfig
	AuditStream  AuditStreamConfig
}

// AccessConfig configures access control: policy engine, subject cache, bootstrap admins, assignable roles.
type AccessConfig struct {
	Engine               RBACEngineKind
	CacheTTL             time.Duration
	BootstrapAdminEmails []string
	AssignableRoles      []string
}

// IsLocal reports whether the process runs in the local development environment.
func (c Config) IsLocal() bool {
	return c.Environment == EnvironmentLocal
}

// HTTPConfig configures the service's public HTTP listener and its lifecycle.
type HTTPConfig struct {
	Addr              string
	PublicURL         string
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
	CursorSecret      string
}

// InternalConfig configures the optional internal listener used by workloads (api only).
type InternalConfig struct {
	Addr          string
	WorkloadAuth  WorkloadAuthKind
	WorkloadToken string
	K8s           K8sReviewConfig
	MTLSAllowed   []string
}

// K8sReviewConfig points at the Kubernetes TokenReview API used to verify workload tokens.
type K8sReviewConfig struct {
	APIServerURL  string
	ReviewerToken string
	CAFile        string
	Audience      string
}

// TLSConfig holds listener certificates; empty means plain HTTP (local only).
type TLSConfig struct {
	CertFile     string
	KeyFile      string
	ClientCAFile string
}

// LogConfig configures structured logging.
type LogConfig struct {
	Level slog.Level
}

// DatabaseConfig points at the PostgreSQL metadata store.
// PoolMaxConns 0 keeps the store default; a pool_max_conns DSN parameter wins over both.
type DatabaseConfig struct {
	URL            string
	MigrateOnStart bool
	PoolMaxConns   int
}

// RedisConfig points at the short-lived data store (sessions, samples, live tail).
type RedisConfig struct {
	URL string
}

// KafkaConfig holds the platform's own Kafka bootstrap (local dev / internal topics).
type KafkaConfig struct {
	Bootstrap string
}

// SecretsConfig selects the secrets provider implementation and its settings.
type SecretsConfig struct {
	Provider     SecretsProviderKind
	LocalKeyFile string
	Static       StaticKeyConfig
	Rotation     SecretsRotationConfig
	Vault        VaultConfig
	KMS          KMSConfig
}

// VaultConfig configures the Vault Transit master-key provider.
type VaultConfig struct {
	Addr       string
	TransitKey string
	Token      string
	Mount      string
}

// AuthConfig configures interactive login, sessions and authorization.
type AuthConfig struct {
	Modes          []AuthMode
	Authz          AuthzMode
	CookieSecret   string
	CookieSecure   bool
	SessionIdleTTL time.Duration
	SessionMaxTTL  time.Duration
	OIDC           OIDCConfig
	Dev            DevLoginConfig
	Password       PasswordLoginConfig
}

// HasMode reports whether the given authentication mode is enabled.
func (a AuthConfig) HasMode(mode AuthMode) bool {
	for _, m := range a.Modes {
		if m == mode {
			return true
		}
	}
	return false
}

// OIDCConfig configures the generic OpenID Connect login.
type OIDCConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
	RoleClaims   OIDCRoleClaimConfig
}

// DevLoginConfig configures the local-only dev login seed.
type DevLoginConfig struct {
	AdminEmail string
	TeamName   string
}

// ControlPlaneConfig tells workers and preview where the API (and its internal listener) lives.
type ControlPlaneConfig struct {
	URL            string
	InternalURL    string
	CAFile         string
	WorkloadToken  string
	TokenFile      string
	ClientCertFile string
	ClientKeyFile  string
}
