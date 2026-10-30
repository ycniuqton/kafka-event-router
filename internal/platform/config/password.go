package config

import "time"

// Password-login environment keys (access-control 00001-password-auth).
const (
	EnvBootstrapAdminEmail        EnvKey = "ROUTER_BOOTSTRAP_ADMIN_EMAIL"
	EnvBootstrapAdminPassword     EnvKey = "ROUTER_BOOTSTRAP_ADMIN_PASSWORD"
	EnvBootstrapAdminPasswordFile EnvKey = "ROUTER_BOOTSTRAP_ADMIN_PASSWORD_FILE"
	EnvLoginMaxFailures           EnvKey = "ROUTER_LOGIN_MAX_FAILURES"
	EnvLoginLockout               EnvKey = "ROUTER_LOGIN_LOCKOUT"
	EnvLoginIPPerMinute           EnvKey = "ROUTER_LOGIN_IP_PER_MINUTE"
)

// AuthModePassword enables email + password login against credentials stored in Postgres (any environment).
const AuthModePassword AuthMode = "password"

// LoginThrottleNumbers groups numeric fallbacks of the password-login throttle.
type LoginThrottleNumbers int

const (
	DefaultLoginMaxFailures LoginThrottleNumbers = 5
	DefaultLoginIPPerMinute LoginThrottleNumbers = 20
)

// LoginThrottleDurations groups duration fallbacks of the password-login throttle.
type LoginThrottleDurations time.Duration

const DefaultLoginLockout LoginThrottleDurations = LoginThrottleDurations(15 * time.Minute)

// PasswordLoginConfig configures password login: the first-start admin and the failed-login throttle.
type PasswordLoginConfig struct {
	BootstrapEmail        string
	BootstrapPassword     string
	BootstrapPasswordFile string
	MaxFailures           int
	Lockout               time.Duration
	IPPerMinute           int
}

// extendPassword fills the password-login settings of cfg.
func (l *Loader) extendPassword(cfg *Config) error {
	failures, err := l.integer(EnvLoginMaxFailures, int(DefaultLoginMaxFailures))
	if err != nil {
		return err
	}
	lockout, err := l.duration(EnvLoginLockout, time.Duration(DefaultLoginLockout))
	if err != nil {
		return err
	}
	perIP, err := l.integer(EnvLoginIPPerMinute, int(DefaultLoginIPPerMinute))
	if err != nil {
		return err
	}
	cfg.Auth.Password = PasswordLoginConfig{
		BootstrapEmail:        l.str(EnvBootstrapAdminEmail, ""),
		BootstrapPassword:     l.str(EnvBootstrapAdminPassword, ""),
		BootstrapPasswordFile: l.str(EnvBootstrapAdminPasswordFile, ""),
		MaxFailures:           failures, Lockout: lockout, IPPerMinute: perIP,
	}
	return nil
}
