package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// LookupFunc resolves an environment key; it matches os.LookupEnv.
type LookupFunc func(key string) (string, bool)

// Loader builds a Config from environment variables with typed defaults and validates it.
type Loader struct {
	lookup LookupFunc
	guard  *EnvironmentGuard
}

// NewLoader returns a Loader reading from the process environment.
func NewLoader() *Loader {
	return NewLoaderWithLookup(os.LookupEnv)
}

// NewLoaderWithLookup returns a Loader backed by a custom lookup (useful in tests).
func NewLoaderWithLookup(lookup LookupFunc) *Loader {
	return &Loader{lookup: lookup, guard: NewEnvironmentGuard()}
}

// Load assembles and validates the configuration for the given service.
func (l *Loader) Load(service ServiceName) (Config, error) {
	service = ServiceName(l.str(EnvService, string(service)))
	env := Environment(l.str(EnvEnvironment, string(EnvironmentLocal)))
	level, err := l.logLevel()
	if err != nil {
		return Config{}, err
	}
	httpCfg, err := l.httpConfig(service, env)
	if err != nil {
		return Config{}, err
	}
	authCfg, err := l.authConfig(env)
	if err != nil {
		return Config{}, err
	}
	migrate, err := l.boolean(EnvMigrateOnStart, false)
	if err != nil {
		return Config{}, err
	}
	accessCfg, err := l.accessConfig()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Service:      service,
		Environment:  env,
		Workspace:    l.str(EnvWorkspace, string(DefaultWorkspace)),
		HTTP:         httpCfg,
		Internal:     l.internalConfig(env),
		TLS:          l.tlsConfig(),
		Log:          LogConfig{Level: level},
		Database:     DatabaseConfig{URL: l.str(EnvDatabaseURL, string(DefaultDatabaseURL)), MigrateOnStart: migrate},
		Redis:        RedisConfig{URL: l.str(EnvRedisURL, string(DefaultRedisURL))},
		Kafka:        KafkaConfig{Bootstrap: l.str(EnvKafkaBootstrap, string(DefaultKafkaBootstrap))},
		Secrets:      l.secretsConfig(env),
		Auth:         authCfg,
		Access:       accessCfg,
		ControlPlane: l.controlPlaneConfig(),
	}
	if err := l.extend(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, l.guard.Validate(cfg)
}

func (l *Loader) httpConfig(service ServiceName, env Environment) (HTTPConfig, error) {
	readHeader, err := l.duration(EnvHTTPReadHeader, time.Duration(DefaultReadHeaderTimeout))
	if err != nil {
		return HTTPConfig{}, err
	}
	shutdown, err := l.duration(EnvHTTPShutdown, time.Duration(DefaultShutdownTimeout))
	if err != nil {
		return HTTPConfig{}, err
	}
	return HTTPConfig{
		Addr:              l.str(EnvHTTPAddr, service.DefaultAddr()),
		PublicURL:         l.str(EnvPublicURL, string(DefaultPublicURL)),
		ReadHeaderTimeout: readHeader,
		ShutdownTimeout:   shutdown,
		CursorSecret:      l.str(EnvCursorSecret, l.localSecretDefault(env)),
	}, nil
}

func (l *Loader) authConfig(env Environment) (AuthConfig, error) {
	idle, err := l.duration(EnvSessionIdleTTL, time.Duration(DefaultSessionIdleTTL))
	if err != nil {
		return AuthConfig{}, err
	}
	maxTTL, err := l.duration(EnvSessionAbsTTL, time.Duration(DefaultSessionMaxTTL))
	if err != nil {
		return AuthConfig{}, err
	}
	secure, err := l.boolean(EnvCookieSecure, env != EnvironmentLocal)
	if err != nil {
		return AuthConfig{}, err
	}
	modes := []AuthMode{}
	for _, m := range l.list(EnvAuthModes, l.byEnv(env, DefaultAuthModes, DefaultRemoteAuthModes)) {
		modes = append(modes, AuthMode(m))
	}
	return AuthConfig{
		Modes:          modes,
		Authz:          AuthzMode(l.str(EnvAuthz, string(DefaultAuthz))),
		CookieSecret:   l.str(EnvCookieSecret, l.localSecretDefault(env)),
		CookieSecure:   secure,
		SessionIdleTTL: idle,
		SessionMaxTTL:  maxTTL,
		OIDC: OIDCConfig{
			Issuer:       l.str(EnvOIDCIssuer, ""),
			ClientID:     l.str(EnvOIDCClientID, ""),
			ClientSecret: l.str(EnvOIDCClientSecret, ""),
			RedirectURL:  l.str(EnvOIDCRedirectURL, ""),
			Scopes:       l.list(EnvOIDCScopes, string(DefaultOIDCScopes)),
		},
		Dev: DevLoginConfig{
			AdminEmail: l.str(EnvDevAdminEmail, string(DefaultDevAdminEmail)),
			TeamName:   l.str(EnvDevTeamName, string(DefaultDevTeamName)),
		},
	}, nil
}

func (l *Loader) accessConfig() (AccessConfig, error) {
	ttl, err := l.duration(EnvRBACCacheTTL, time.Duration(DefaultRBACCacheTTL))
	if err != nil {
		return AccessConfig{}, err
	}
	return AccessConfig{
		Engine:               RBACEngineKind(l.str(EnvRBACEngine, string(DefaultRBACEngine))),
		CacheTTL:             ttl,
		BootstrapAdminEmails: l.list(EnvBootstrapAdminEmails, ""),
		AssignableRoles:      l.list(EnvRBACAssignableRoles, string(DefaultRBACAssignableRoles)),
	}, nil
}

func (l *Loader) internalConfig(env Environment) InternalConfig {
	return InternalConfig{
		Addr:          l.str(EnvInternalAddr, ""),
		WorkloadAuth:  WorkloadAuthKind(l.str(EnvWorkloadAuth, l.byEnv(env, DefaultWorkloadAuth, DefaultRemoteWorkloadAuth))),
		WorkloadToken: l.str(EnvWorkloadToken, ""),
		K8s: K8sReviewConfig{
			APIServerURL:  l.str(EnvK8sAPIServerURL, string(DefaultK8sAPIServerURL)),
			ReviewerToken: l.str(EnvK8sReviewerToken, ""),
			CAFile:        l.str(EnvK8sCAFile, string(DefaultK8sCAFile)),
			Audience:      l.str(EnvWorkloadAudience, string(DefaultWorkloadAudience)),
		},
	}
}

func (l *Loader) tlsConfig() TLSConfig {
	return TLSConfig{
		CertFile:     l.str(EnvTLSCertFile, ""),
		KeyFile:      l.str(EnvTLSKeyFile, ""),
		ClientCAFile: l.str(EnvTLSClientCAFile, ""),
	}
}

func (l *Loader) secretsConfig(env Environment) SecretsConfig {
	return SecretsConfig{
		Provider:     SecretsProviderKind(l.str(EnvSecretsProvider, l.byEnv(env, DefaultSecretsProvider, DefaultRemoteSecrets))),
		LocalKeyFile: l.str(EnvSecretsLocalKeyFile, string(DefaultLocalKeyFile)),
		Vault: VaultConfig{
			Addr:       l.str(EnvVaultAddr, ""),
			TransitKey: l.str(EnvVaultTransitKey, string(DefaultVaultTransitKey)),
			Token:      l.str(EnvVaultToken, ""),
			Mount:      l.str(EnvVaultMount, string(DefaultVaultMount)),
		},
	}
}

func (l *Loader) controlPlaneConfig() ControlPlaneConfig {
	return ControlPlaneConfig{
		URL:           l.str(EnvControlPlaneURL, string(DefaultControlPlaneURL)),
		InternalURL:   l.str(EnvControlPlaneIntURL, string(DefaultControlPlaneInt)),
		CAFile:        l.str(EnvControlPlaneCAFile, ""),
		WorkloadToken: l.str(EnvWorkloadToken, ""),
		TokenFile:     l.str(EnvWorkloadTokenFile, ""),
	}
}

func (l *Loader) localSecretDefault(env Environment) string {
	return l.byEnv(env, DefaultLocalDevSecret, "")
}

func (l *Loader) byEnv(env Environment, local, remote ConnectionDefaults) string {
	if env == EnvironmentLocal {
		return string(local)
	}
	return string(remote)
}

func (l *Loader) logLevel() (slog.Level, error) {
	var level slog.Level
	raw := l.str(EnvLogLevel, string(DefaultLogLevel))
	if err := level.UnmarshalText([]byte(raw)); err != nil {
		return level, fmt.Errorf("config: %s: %w", EnvLogLevel, err)
	}
	return level, nil
}

func (l *Loader) str(key EnvKey, fallback string) string {
	if v, ok := l.lookup(string(key)); ok && v != "" {
		return v
	}
	return fallback
}

func (l *Loader) list(key EnvKey, fallback string) []string {
	items := []string{}
	for _, part := range strings.Split(l.str(key, fallback), string(CommaSeparator)) {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func (l *Loader) boolean(key EnvKey, fallback bool) (bool, error) {
	raw, ok := l.lookup(string(key))
	if !ok || raw == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("config: %s: %w", key, err)
	}
	return b, nil
}

func (l *Loader) duration(key EnvKey, fallback time.Duration) (time.Duration, error) {
	raw, ok := l.lookup(string(key))
	if !ok || raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s: %w", key, err)
	}
	return d, nil
}
