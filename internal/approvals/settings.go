package approvals

import "os"

// SettingKey names the approvals environment settings.
type SettingKey string

const (
	EnvPolicy   SettingKey = "ROUTER_APPROVALS_POLICY"
	EnvNotifier SettingKey = "ROUTER_APPROVALS_NOTIFIER"
)

// Settings select the gate policy and the notification transport.
type Settings struct {
	Policy   PolicyKind
	Notifier NotifierKind
}

// Lookup reads one environment value.
type Lookup func(key string) (string, bool)

// SettingsLoader reads Settings from the environment with defaults (team_production, log).
type SettingsLoader struct {
	Lookup Lookup
}

// NewSettingsLoader reads the process environment.
func NewSettingsLoader() SettingsLoader {
	return SettingsLoader{Lookup: os.LookupEnv}
}

// Load returns the settings.
func (l SettingsLoader) Load() Settings {
	return Settings{Policy: PolicyKind(l.str(EnvPolicy, string(PolicyTeamProduction))), Notifier: NotifierKind(l.str(EnvNotifier, string(NotifierLog)))}
}

func (l SettingsLoader) str(key SettingKey, fallback string) string {
	if v, ok := l.Lookup(string(key)); ok && v != "" {
		return v
	}
	return fallback
}
