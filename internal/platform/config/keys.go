package config

import "time"

// EnvKey names an environment variable understood by the configuration loader.
type EnvKey string

const (
	EnvService         EnvKey = "ROUTER_SERVICE"
	EnvEnvironment     EnvKey = "ROUTER_ENV"
	EnvWorkspace       EnvKey = "ROUTER_WORKSPACE_NAME"
	EnvHTTPAddr        EnvKey = "ROUTER_HTTP_ADDR"
	EnvHTTPReadHeader  EnvKey = "ROUTER_HTTP_READ_HEADER_TIMEOUT"
	EnvHTTPShutdown    EnvKey = "ROUTER_HTTP_SHUTDOWN_TIMEOUT"
	EnvPublicURL       EnvKey = "ROUTER_PUBLIC_URL"
	EnvCursorSecret    EnvKey = "ROUTER_CURSOR_SECRET"
	EnvLogLevel        EnvKey = "ROUTER_LOG_LEVEL"
	EnvDatabaseURL     EnvKey = "ROUTER_DATABASE_URL"
	EnvDBPoolMaxConns  EnvKey = "ROUTER_DB_POOL_MAX_CONNS"
	EnvMigrateOnStart  EnvKey = "ROUTER_MIGRATE_ON_START"
	EnvRedisURL        EnvKey = "ROUTER_REDIS_URL"
	EnvKafkaBootstrap  EnvKey = "ROUTER_KAFKA_BOOTSTRAP"
	EnvControlPlaneURL EnvKey = "ROUTER_CONTROL_PLANE_URL"

	EnvInternalAddr        EnvKey = "ROUTER_INTERNAL_ADDR"
	EnvWorkloadAuth        EnvKey = "ROUTER_WORKLOAD_AUTH"
	EnvWorkloadToken       EnvKey = "ROUTER_WORKLOAD_TOKEN"
	EnvK8sAPIServerURL     EnvKey = "ROUTER_K8S_API_URL"
	EnvK8sReviewerToken    EnvKey = "ROUTER_K8S_REVIEWER_TOKEN"
	EnvK8sCAFile           EnvKey = "ROUTER_K8S_CA_FILE"
	EnvWorkloadAudience    EnvKey = "ROUTER_WORKLOAD_AUDIENCE"
	EnvControlPlaneIntURL  EnvKey = "ROUTER_CONTROL_PLANE_INTERNAL_URL"
	EnvControlPlaneCAFile  EnvKey = "ROUTER_CONTROL_PLANE_CA_FILE"
	EnvWorkloadTokenFile   EnvKey = "ROUTER_WORKLOAD_TOKEN_FILE"
	EnvTLSCertFile         EnvKey = "ROUTER_TLS_CERT_FILE"
	EnvTLSKeyFile          EnvKey = "ROUTER_TLS_KEY_FILE"
	EnvTLSClientCAFile     EnvKey = "ROUTER_TLS_CLIENT_CA_FILE"
	EnvSecretsProvider     EnvKey = "ROUTER_SECRETS_PROVIDER"
	EnvSecretsLocalKeyFile EnvKey = "ROUTER_SECRETS_LOCAL_KEY_FILE"
	EnvVaultAddr           EnvKey = "ROUTER_VAULT_ADDR"
	EnvVaultTransitKey     EnvKey = "ROUTER_VAULT_TRANSIT_KEY"
	EnvVaultToken          EnvKey = "ROUTER_VAULT_TOKEN"
	EnvVaultMount          EnvKey = "ROUTER_VAULT_TRANSIT_MOUNT"

	EnvAuthModes        EnvKey = "ROUTER_AUTH_MODES"
	EnvAuthz            EnvKey = "ROUTER_AUTHZ"
	EnvCookieSecret     EnvKey = "ROUTER_COOKIE_SECRET"
	EnvCookieSecure     EnvKey = "ROUTER_COOKIE_SECURE"
	EnvSessionIdleTTL   EnvKey = "ROUTER_SESSION_IDLE_TTL"
	EnvSessionAbsTTL    EnvKey = "ROUTER_SESSION_ABSOLUTE_TTL"
	EnvOIDCIssuer       EnvKey = "ROUTER_OIDC_ISSUER"
	EnvOIDCClientID     EnvKey = "ROUTER_OIDC_CLIENT_ID"
	EnvOIDCClientSecret EnvKey = "ROUTER_OIDC_CLIENT_SECRET"
	EnvOIDCRedirectURL  EnvKey = "ROUTER_OIDC_REDIRECT_URL"
	EnvOIDCScopes       EnvKey = "ROUTER_OIDC_SCOPES"
	EnvDevAdminEmail    EnvKey = "ROUTER_DEV_ADMIN_EMAIL"
	EnvDevTeamName      EnvKey = "ROUTER_DEV_TEAM_NAME"

	EnvRBACEngine           EnvKey = "ROUTER_RBAC_ENGINE"
	EnvRBACCacheTTL         EnvKey = "ROUTER_RBAC_CACHE_TTL"
	EnvRBACAssignableRoles  EnvKey = "ROUTER_RBAC_ASSIGNABLE_ROLES"
	EnvBootstrapAdminEmails EnvKey = "ROUTER_BOOTSTRAP_ADMIN_EMAILS"
)

// ServiceName identifies which deployable binary is running.
type ServiceName string

const (
	ServiceAPI     ServiceName = "api"
	ServiceWorker  ServiceName = "worker"
	ServicePreview ServiceName = "preview"
)

// Environment is the deployment stage the process runs in.
type Environment string

const (
	EnvironmentLocal      Environment = "local"
	EnvironmentStaging    Environment = "staging"
	EnvironmentProduction Environment = "production"
)

// AuthMode names an authentication mode enabled through ROUTER_AUTH_MODES.
type AuthMode string

const (
	AuthModeOIDC     AuthMode = "oidc"
	AuthModeDev      AuthMode = "dev"
	AuthModeAPIToken AuthMode = "api_token"
)

// GuardedSetting names a non-mode setting value the environment guard restricts.
type GuardedSetting string

const (
	SettingInsecureCookies     GuardedSetting = "ROUTER_COOKIE_SECURE=false"
	SettingInternalWithoutMTLS GuardedSetting = "internal listener without client certificates (ROUTER_TLS_CLIENT_CA_FILE)"
)

// AuthzMode selects the Authorizer implementation (bootstrap until access-control registers rbac).
type AuthzMode string

const (
	AuthzBootstrap AuthzMode = "bootstrap"
	AuthzRBAC      AuthzMode = "rbac"
)

// RBACEngineKind selects the access-control policy engine implementation (ROUTER_RBAC_ENGINE).
type RBACEngineKind string

const (
	RBACEngineRoleBinding RBACEngineKind = "rolebinding"
	RBACEngineOPA         RBACEngineKind = "opa"
)

// AccessDefaults groups fallback values of the access-control settings.
type AccessDefaults string

const (
	DefaultRBACEngine          AccessDefaults = "rolebinding"
	DefaultRBACAssignableRoles AccessDefaults = "viewer,editor,approver,admin"
)

// AccessCacheDefaults groups fallback durations of the access-control subject cache.
type AccessCacheDefaults time.Duration

const DefaultRBACCacheTTL AccessCacheDefaults = AccessCacheDefaults(30 * time.Second)

// WorkloadAuthKind selects how worker/preview workloads authenticate to the internal listener.
type WorkloadAuthKind string

const (
	WorkloadStaticToken WorkloadAuthKind = "static_token"
	WorkloadK8sToken    WorkloadAuthKind = "k8s_token"
	WorkloadMTLS        WorkloadAuthKind = "mtls"
)

// SecretsProviderKind names a master-key provider implementation.
type SecretsProviderKind string

const (
	SecretsLocal SecretsProviderKind = "local"
	SecretsVault SecretsProviderKind = "vault"
	SecretsKMS   SecretsProviderKind = "kms"
)

// ListSeparator splits comma-separated list values in env variables.
type ListSeparator string

const CommaSeparator ListSeparator = ","

// HTTPDefaults groups fallback values for the HTTP server settings.
type HTTPDefaults time.Duration

const (
	DefaultReadHeaderTimeout HTTPDefaults = HTTPDefaults(5 * time.Second)
	DefaultShutdownTimeout   HTTPDefaults = HTTPDefaults(15 * time.Second)
)

// SessionDefaults groups fallback session lifetimes.
type SessionDefaults time.Duration

const (
	DefaultSessionIdleTTL SessionDefaults = SessionDefaults(8 * time.Hour)
	DefaultSessionMaxTTL  SessionDefaults = SessionDefaults(7 * 24 * time.Hour)
)

// ConnectionDefaults groups fallback endpoints and names for local development.
type ConnectionDefaults string

const (
	DefaultDatabaseURL        ConnectionDefaults = "postgres://router:router@localhost:5432/router?sslmode=disable"
	DefaultRedisURL           ConnectionDefaults = "redis://localhost:6379/0"
	DefaultKafkaBootstrap     ConnectionDefaults = "localhost:9092"
	DefaultControlPlaneURL    ConnectionDefaults = "http://localhost:8080"
	DefaultControlPlaneInt    ConnectionDefaults = "http://localhost:8443"
	DefaultPublicURL          ConnectionDefaults = "http://localhost:8080"
	DefaultSecretsProvider    ConnectionDefaults = "local"
	DefaultLogLevel           ConnectionDefaults = "info"
	DefaultWorkspace          ConnectionDefaults = "default"
	DefaultAuthModes          ConnectionDefaults = "dev,oidc,api_token"
	DefaultRemoteAuthModes    ConnectionDefaults = "oidc,api_token"
	DefaultRemoteSecrets      ConnectionDefaults = "vault"
	DefaultRemoteWorkloadAuth ConnectionDefaults = "k8s_token"
	DefaultAuthz              ConnectionDefaults = "rbac"
	DefaultWorkloadAuth       ConnectionDefaults = "static_token"
	DefaultLocalKeyFile       ConnectionDefaults = "./.local/master.key"
	DefaultVaultTransitKey    ConnectionDefaults = "event-router"
	DefaultVaultMount         ConnectionDefaults = "transit"
	DefaultOIDCScopes         ConnectionDefaults = "openid,email,profile"
	DefaultDevAdminEmail      ConnectionDefaults = "admin@local.dev"
	DefaultDevTeamName        ConnectionDefaults = "default"
	DefaultWorkloadAudience   ConnectionDefaults = "event-router-internal"
	DefaultK8sAPIServerURL    ConnectionDefaults = "https://kubernetes.default.svc"
	DefaultK8sCAFile          ConnectionDefaults = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	DefaultLocalDevSecret     ConnectionDefaults = "local-dev-only-secret-change-me"
)

var defaultServiceAddrs = map[ServiceName]string{
	ServiceAPI:     ":8080",
	ServiceWorker:  ":8081",
	ServicePreview: ":8082",
}

// DefaultAddr returns the listen address used when none is configured.
func (s ServiceName) DefaultAddr() string {
	return defaultServiceAddrs[s]
}
