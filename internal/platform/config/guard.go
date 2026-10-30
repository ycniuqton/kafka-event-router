package config

import (
	"errors"
	"fmt"
	"slices"
)

// GuardRule is one environment restriction checked after loading.
type GuardRule interface {
	Check(cfg Config) error
}

// EnvironmentGuard rejects configurations that use local-only modes outside local or miss required secrets.
type EnvironmentGuard struct {
	rules []GuardRule
}

// NewEnvironmentGuard returns the guard with the platform's standard rules.
func NewEnvironmentGuard() *EnvironmentGuard {
	return &EnvironmentGuard{rules: []GuardRule{
		LocalOnlyRule{Mode: string(AuthModeDev), Used: func(c Config) bool { return c.Auth.HasMode(AuthModeDev) }},
		LocalOnlyRule{Mode: string(SecretsLocal), Used: func(c Config) bool { return c.Secrets.Provider == SecretsLocal }},
		staticKeyRule(),
		EnvironmentScopedRule{Mode: string(WorkloadStaticToken), Allowed: NonProductionEnvironments,
			Used: func(c Config) bool { return c.Internal.Addr != "" && c.Internal.WorkloadAuth == WorkloadStaticToken }},
		EnvironmentScopedRule{Mode: string(SettingInsecureCookies), Allowed: NonProductionEnvironments,
			Used: func(c Config) bool { return !c.Auth.CookieSecure }},
		EnvironmentScopedRule{Mode: string(SettingInternalWithoutMTLS), Allowed: NonProductionEnvironments,
			Used: func(c Config) bool { return c.Internal.Addr != "" && !c.TLS.RequiresClientCerts() }},
		ConditionalValueRule{Key: EnvTLSClientCAFile, Reason: string(WorkloadMTLS),
			When:  func(c Config) bool { return c.Internal.Addr != "" && c.Internal.WorkloadAuth == WorkloadMTLS },
			Value: func(c Config) string { return c.TLS.ClientCAFile }},
		ConditionalValueRule{Key: EnvTLSCertFile, Reason: string(EnvTLSClientCAFile),
			When: func(c Config) bool { return c.TLS.ClientCAFile != "" }, Value: func(c Config) string { return c.TLS.CertFile }},
		ConditionalValueRule{Key: EnvTLSKeyFile, Reason: string(EnvTLSCertFile),
			When: func(c Config) bool { return c.TLS.CertFile != "" }, Value: func(c Config) string { return c.TLS.KeyFile }},
		RequiredValueRule{Service: ServiceAPI, Key: EnvCursorSecret, Value: func(c Config) string { return c.HTTP.CursorSecret }},
		RequiredValueRule{Service: ServiceAPI, Key: EnvCookieSecret, Value: func(c Config) string { return c.Auth.CookieSecret }},
	}}
}

// Validate runs every rule and joins all violations.
func (g *EnvironmentGuard) Validate(cfg Config) error {
	var errs []error
	for _, rule := range g.rules {
		if err := rule.Check(cfg); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// LocalOnlyRule forbids a mode outside the local environment.
type LocalOnlyRule struct {
	Mode string
	Used func(Config) bool
}

// Check fails when the mode is used and the environment is not local.
func (r LocalOnlyRule) Check(cfg Config) error {
	if r.Used(cfg) && !cfg.IsLocal() {
		return fmt.Errorf("config: %s is only allowed in local environment", r.Mode)
	}
	return nil
}

// NonProductionEnvironments may run single-host setups without k8s workload identity or TLS.
var NonProductionEnvironments = []Environment{EnvironmentLocal, EnvironmentStaging}

// EnvironmentScopedRule forbids a mode outside an explicit set of environments.
type EnvironmentScopedRule struct {
	Mode    string
	Allowed []Environment
	Used    func(Config) bool
}

// Check fails when the mode is used and the environment is not in Allowed.
func (r EnvironmentScopedRule) Check(cfg Config) error {
	if r.Used(cfg) && !slices.Contains(r.Allowed, cfg.Environment) {
		return fmt.Errorf("config: %s is only allowed in %v environments", r.Mode, r.Allowed)
	}
	return nil
}

// RequiredValueRule demands a non-empty value (local env gets dev defaults from the loader).
type RequiredValueRule struct {
	Service ServiceName
	Key     EnvKey
	Value   func(Config) string
}

// Check fails when the value is empty for the service the rule applies to.
func (r RequiredValueRule) Check(cfg Config) error {
	if cfg.Service == r.Service && r.Value(cfg) == "" {
		return fmt.Errorf("config: %s is required", r.Key)
	}
	return nil
}

// ConditionalValueRule demands a value only when a setting that depends on it is active.
type ConditionalValueRule struct {
	Key    EnvKey
	Reason string
	When   func(Config) bool
	Value  func(Config) string
}

// Check fails when the condition holds and the value is empty.
func (r ConditionalValueRule) Check(cfg Config) error {
	if r.When(cfg) && r.Value(cfg) == "" {
		return fmt.Errorf("config: %s is required for %s", r.Key, r.Reason)
	}
	return nil
}
