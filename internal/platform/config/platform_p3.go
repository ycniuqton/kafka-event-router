package config

import "time"

// P3 environment keys of platform-core: master-key rotation, OIDC claim → role mapping, audit streaming.
const (
	EnvSecretsLocalPreviousKeys EnvKey = "ROUTER_SECRETS_LOCAL_PREVIOUS_KEY_FILES"
	EnvSecretsRotationInterval  EnvKey = "ROUTER_SECRETS_ROTATION_INTERVAL"
	EnvSecretsRotationPageSize  EnvKey = "ROUTER_SECRETS_ROTATION_PAGE_SIZE"
	EnvOIDCRoleClaim            EnvKey = "ROUTER_OIDC_ROLE_CLAIM"
	EnvOIDCRoleMappings         EnvKey = "ROUTER_OIDC_ROLE_MAPPINGS"
	EnvAuditStreamSinks         EnvKey = "ROUTER_AUDIT_STREAM_SINKS"
	EnvAuditStreamBuffer        EnvKey = "ROUTER_AUDIT_STREAM_BUFFER"
)

// RotationDefaults groups the fallback schedule of the master-key rotation job.
type RotationDefaults time.Duration

const DefaultSecretsRotationInterval RotationDefaults = RotationDefaults(24 * time.Hour)

// P3Sizes groups the fallback sizes of P3 settings.
type P3Sizes int

const (
	DefaultRotationPageSize  P3Sizes = 100
	DefaultAuditStreamBuffer P3Sizes = 1024
)

// P3Defaults groups fallback strings of P3 settings.
type P3Defaults string

const DefaultOIDCRoleClaim P3Defaults = "groups"

// SecretsRotationConfig drives master-key rotation: decrypt-only previous local keys and the leader job schedule.
type SecretsRotationConfig struct {
	LocalPreviousKeyFiles []string
	Interval              time.Duration
	PageSize              int
}

// Enabled reports whether the periodic rotation job runs (interval 0 disables it; the CLI always works).
func (c SecretsRotationConfig) Enabled() bool {
	return c.Interval > 0
}

// OIDCRoleClaimConfig maps IdP groups (read from Claim, a dot path) to team roles: "group=team:role" entries.
type OIDCRoleClaimConfig struct {
	Claim    string
	Mappings []string
}

// Enabled reports whether any group mapping is configured.
func (c OIDCRoleClaimConfig) Enabled() bool {
	return len(c.Mappings) > 0
}

// AuditStreamKind names a real-time audit sink implementation (ROUTER_AUDIT_STREAM_SINKS).
type AuditStreamKind string

const AuditStreamLog AuditStreamKind = "log"

// AuditStreamConfig selects the sinks that receive every committed audit entry and their buffer size.
type AuditStreamConfig struct {
	Sinks  []AuditStreamKind
	Buffer int
}

// extendP3 fills the P3 settings of cfg.
func (l *Loader) extendP3(cfg *Config) error {
	interval, err := l.duration(EnvSecretsRotationInterval, time.Duration(DefaultSecretsRotationInterval))
	if err != nil {
		return err
	}
	page, err := l.integer(EnvSecretsRotationPageSize, int(DefaultRotationPageSize))
	if err != nil {
		return err
	}
	buffer, err := l.integer(EnvAuditStreamBuffer, int(DefaultAuditStreamBuffer))
	if err != nil {
		return err
	}
	cfg.Secrets.Rotation = SecretsRotationConfig{LocalPreviousKeyFiles: l.list(EnvSecretsLocalPreviousKeys, ""),
		Interval: interval, PageSize: page}
	cfg.Auth.OIDC.RoleClaims = OIDCRoleClaimConfig{Claim: l.str(EnvOIDCRoleClaim, string(DefaultOIDCRoleClaim)),
		Mappings: l.list(EnvOIDCRoleMappings, "")}
	cfg.AuditStream = AuditStreamConfig{Buffer: buffer}
	for _, s := range l.list(EnvAuditStreamSinks, "") {
		cfg.AuditStream.Sinks = append(cfg.AuditStream.Sinks, AuditStreamKind(s))
	}
	return nil
}
