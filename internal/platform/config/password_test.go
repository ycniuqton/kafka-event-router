package config

import (
	"testing"
	"time"
)

func TestPasswordModeAllowedInProductionWithoutOIDC(t *testing.T) {
	env := map[string]string{"ROUTER_ENV": "production", "ROUTER_AUTH_MODES": "password,api_token", "ROUTER_COOKIE_SECRET": "c",
		"ROUTER_CURSOR_SECRET": "k", "ROUTER_BOOTSTRAP_ADMIN_EMAIL": "admin@example.com", "ROUTER_BOOTSTRAP_ADMIN_PASSWORD_FILE": "/run/pw",
		"ROUTER_LOGIN_MAX_FAILURES": "7", "ROUTER_LOGIN_LOCKOUT": "5m", "ROUTER_SECRETS_PROVIDER": "aws_kms"}
	cfg, err := NewLoaderWithLookup(func(k string) (string, bool) { v, ok := env[k]; return v, ok }).Load(ServiceAPI)
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Auth.Password
	if !cfg.Auth.HasMode(AuthModePassword) || cfg.Auth.HasMode(AuthModeOIDC) || p.BootstrapEmail != "admin@example.com" ||
		p.BootstrapPasswordFile != "/run/pw" || p.MaxFailures != 7 || p.Lockout != 5*time.Minute || p.IPPerMinute != int(DefaultLoginIPPerMinute) {
		t.Fatalf("auth %+v", cfg.Auth)
	}
}

func TestPasswordThrottleRejectsBadValues(t *testing.T) {
	env := map[string]string{"ROUTER_LOGIN_LOCKOUT": "soon"}
	if _, err := NewLoaderWithLookup(func(k string) (string, bool) { v, ok := env[k]; return v, ok }).Load(ServiceAPI); err == nil {
		t.Fatal("invalid lockout duration must fail")
	}
}
