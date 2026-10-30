package config

import (
	"fmt"
	"strings"
)

// SecretsStaticKey is the master-key provider fed by an operator-supplied 32-byte key (env or file),
// allowed in every environment (single-host "lite" installs without Vault or KMS).
const SecretsStaticKey SecretsProviderKind = "static_key"

// Static master-key environment keys: exactly one of key/key file; previous keys are decrypt-only.
const (
	EnvSecretsMasterKey              EnvKey = "ROUTER_SECRETS_MASTER_KEY"
	EnvSecretsMasterKeyFile          EnvKey = "ROUTER_SECRETS_MASTER_KEY_FILE"
	EnvSecretsPreviousMasterKeys     EnvKey = "ROUTER_SECRETS_PREVIOUS_MASTER_KEYS"
	EnvSecretsPreviousMasterKeyFiles EnvKey = "ROUTER_SECRETS_PREVIOUS_MASTER_KEY_FILES"
)

// StaticKeyConfig holds the base64 master key (inline or file path) and decrypt-only previous keys.
type StaticKeyConfig struct {
	Key              string
	KeyFile          string
	PreviousKeys     []string
	PreviousKeyFiles []string
}

// Sources returns the configured current-key values (inline key, key file), empty when unset.
func (c StaticKeyConfig) Sources() []string {
	return []string{c.Key, c.KeyFile}
}

func (l *Loader) staticKeyConfig() StaticKeyConfig {
	return StaticKeyConfig{
		Key:              l.str(EnvSecretsMasterKey, ""),
		KeyFile:          l.str(EnvSecretsMasterKeyFile, ""),
		PreviousKeys:     l.list(EnvSecretsPreviousMasterKeys, ""),
		PreviousKeyFiles: l.list(EnvSecretsPreviousMasterKeyFiles, ""),
	}
}

// staticKeyRule requires exactly one master-key source on the API when the static_key provider is selected.
func staticKeyRule() GuardRule {
	return ExactlyOneValueRule{Service: ServiceAPI, Keys: []EnvKey{EnvSecretsMasterKey, EnvSecretsMasterKeyFile},
		Reason: string(SecretsStaticKey),
		When:   func(c Config) bool { return c.Secrets.Provider == SecretsStaticKey },
		Values: func(c Config) []string { return c.Secrets.Static.Sources() }}
}

// ExactlyOneValueRule demands that exactly one of several alternative settings is set for a service.
type ExactlyOneValueRule struct {
	Service ServiceName
	Keys    []EnvKey
	Reason  string
	When    func(Config) bool
	Values  func(Config) []string
}

// Check fails when the condition holds for the service and zero or several of the values are set.
func (r ExactlyOneValueRule) Check(cfg Config) error {
	if cfg.Service != r.Service || !r.When(cfg) {
		return nil
	}
	set := 0
	for _, v := range r.Values(cfg) {
		if v != "" {
			set++
		}
	}
	names := make([]string, len(r.Keys))
	for i, k := range r.Keys {
		names[i] = string(k)
	}
	if set != 1 {
		return fmt.Errorf("config: set exactly one of %s for %s", strings.Join(names, " or "), r.Reason)
	}
	return nil
}
