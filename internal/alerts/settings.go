package alerts

import (
	"time"

	"github.com/eventrouter/router/internal/metrics"
	"github.com/eventrouter/router/internal/platform/config"
)

// Configuration keys of alerting (feature-owned).
const (
	EnvAlertsEnabled     config.EnvKey = "ROUTER_ALERTS_ENABLED"
	EnvAlertsInterval    config.EnvKey = "ROUTER_ALERTS_INTERVAL"
	EnvAlertsChannels    config.EnvKey = "ROUTER_ALERTS_CHANNELS"
	EnvAlertsMaxAttempts config.EnvKey = "ROUTER_ALERTS_MAX_ATTEMPTS"
	EnvAlertsSigningKey  config.EnvKey = "ROUTER_ALERTS_WEBHOOK_SIGNING_KEY"
	EnvAlertsHTTPTimeout config.EnvKey = "ROUTER_ALERTS_HTTP_TIMEOUT"
	EnvSMTPHost          config.EnvKey = "ROUTER_SMTP_HOST"
	EnvSMTPPort          config.EnvKey = "ROUTER_SMTP_PORT"
	EnvSMTPUser          config.EnvKey = "ROUTER_SMTP_USER"
	EnvSMTPPassword      config.EnvKey = "ROUTER_SMTP_PASSWORD"
	EnvSMTPFrom          config.EnvKey = "ROUTER_SMTP_FROM"
)

// SettingDefaults are the documented alerting defaults.
type SettingDefaults int64

const (
	DefaultInterval    SettingDefaults = SettingDefaults(30 * time.Second)
	DefaultHTTPTimeout SettingDefaults = SettingDefaults(10 * time.Second)
	DefaultMaxAttempts SettingDefaults = 3
	DefaultSMTPPort    SettingDefaults = 587
)

// SMTPConfig is the ROUTER_SMTP_* block.
type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

// Settings is the alerting configuration.
type Settings struct {
	Enabled     bool
	Interval    time.Duration
	Channels    []ChannelType
	MaxAttempts int
	SigningKey  string
	HTTPTimeout time.Duration
	SMTP        SMTPConfig
}

// SettingsLoader reads Settings through a lookup function (os.LookupEnv in production).
type SettingsLoader struct {
	Lookup config.LookupFunc
}

// Load returns the settings with defaults, rejecting malformed values.
func (l SettingsLoader) Load() (Settings, error) {
	r := &metrics.EnvReader{Lookup: l.Lookup}
	s := Settings{
		Enabled:     r.Bool(EnvAlertsEnabled, true),
		Interval:    r.Duration(EnvAlertsInterval, time.Duration(DefaultInterval)),
		MaxAttempts: int(r.Int(EnvAlertsMaxAttempts, int64(DefaultMaxAttempts))),
		SigningKey:  r.Str(EnvAlertsSigningKey, ""),
		HTTPTimeout: r.Duration(EnvAlertsHTTPTimeout, time.Duration(DefaultHTTPTimeout)),
		SMTP: SMTPConfig{Host: r.Str(EnvSMTPHost, ""), Port: int(r.Int(EnvSMTPPort, int64(DefaultSMTPPort))),
			User: r.Str(EnvSMTPUser, ""), Password: r.Str(EnvSMTPPassword, ""), From: r.Str(EnvSMTPFrom, "")},
	}
	for _, c := range r.List(EnvAlertsChannels) {
		s.Channels = append(s.Channels, ChannelType(c))
	}
	return s, r.Err()
}
