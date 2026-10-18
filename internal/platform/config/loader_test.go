package config

import (
	"strings"
	"testing"
	"time"
)

func lookupFrom(env map[string]string) LookupFunc {
	return func(k string) (string, bool) { v, ok := env[k]; return v, ok }
}

func TestLoaderValidation(t *testing.T) {
	remote := map[string]string{
		"ROUTER_ENV": "production", "ROUTER_CURSOR_SECRET": "c", "ROUTER_COOKIE_SECRET": "k",
	}
	with := func(extra map[string]string) map[string]string {
		out := map[string]string{}
		for k, v := range remote {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	cases := []validationCase{
		{"local defaults ok", ServiceAPI, map[string]string{}, ""},
		{"remote defaults ok", ServiceAPI, remote, ""},
		{"dev auth outside local", ServiceAPI, with(map[string]string{"ROUTER_AUTH_MODES": "oidc,dev"}), "dev is only allowed in local"},
		{"local secrets outside local", ServiceAPI, with(map[string]string{"ROUTER_SECRETS_PROVIDER": "local"}), "local is only allowed in local"},
		{"static workload outside local", ServiceAPI, with(map[string]string{"ROUTER_INTERNAL_ADDR": ":8443", "ROUTER_WORKLOAD_AUTH": "static_token"}), "static_token is only allowed"},
		{"static workload in staging", ServiceAPI, with(map[string]string{"ROUTER_ENV": "staging", "ROUTER_INTERNAL_ADDR": ":8443", "ROUTER_WORKLOAD_AUTH": "static_token"}), ""},
		{"insecure cookies in staging", ServiceAPI, with(map[string]string{"ROUTER_ENV": "staging", "ROUTER_COOKIE_SECURE": "false"}), ""},
		{"insecure cookies in production", ServiceAPI, with(map[string]string{"ROUTER_COOKIE_SECURE": "false"}), "ROUTER_COOKIE_SECURE=false is only allowed"},
		{"local secrets in staging", ServiceAPI, with(map[string]string{"ROUTER_ENV": "staging", "ROUTER_SECRETS_PROVIDER": "local"}), "local is only allowed in local"},
		{"missing cursor secret remote", ServiceAPI, map[string]string{"ROUTER_ENV": "staging", "ROUTER_COOKIE_SECRET": "k"}, "ROUTER_CURSOR_SECRET is required"},
		{"worker needs no api secrets", ServiceWorker, map[string]string{"ROUTER_ENV": "staging"}, ""},
		{"bad duration", ServiceAPI, map[string]string{"ROUTER_SESSION_IDLE_TTL": "soon"}, "ROUTER_SESSION_IDLE_TTL"},
		{"bad bool", ServiceAPI, map[string]string{"ROUTER_MIGRATE_ON_START": "maybe"}, "ROUTER_MIGRATE_ON_START"},
	}
	runValidationCases(t, cases)
}

type validationCase struct {
	name    string
	service ServiceName
	env     map[string]string
	wantErr string
}

func runValidationCases(t *testing.T, cases []validationCase) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewLoaderWithLookup(lookupFrom(tc.env)).Load(tc.service)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestLoaderTransportValidation covers the P2 rules: mTLS on the internal listener, TLS files, tracing, rate limits.
func TestLoaderTransportValidation(t *testing.T) {
	with := func(extra map[string]string) map[string]string {
		out := map[string]string{"ROUTER_ENV": "production", "ROUTER_CURSOR_SECRET": "c", "ROUTER_COOKIE_SECRET": "k"}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	runValidationCases(t, []validationCase{
		{"internal listener needs mTLS in production", ServiceAPI, with(map[string]string{"ROUTER_INTERNAL_ADDR": ":8443"}), "internal listener without client certificates"},
		{"internal mTLS in production", ServiceAPI, with(map[string]string{"ROUTER_INTERNAL_ADDR": ":8443", "ROUTER_WORKLOAD_AUTH": "mtls",
			"ROUTER_TLS_CERT_FILE": "c.pem", "ROUTER_TLS_KEY_FILE": "k.pem", "ROUTER_TLS_CLIENT_CA_FILE": "ca.pem"}), ""},
		{"mtls needs client CA", ServiceAPI, map[string]string{"ROUTER_INTERNAL_ADDR": ":8443", "ROUTER_WORKLOAD_AUTH": "mtls"}, "ROUTER_TLS_CLIENT_CA_FILE is required for mtls"},
		{"client CA needs server cert", ServiceAPI, map[string]string{"ROUTER_TLS_CLIENT_CA_FILE": "ca.pem"}, "ROUTER_TLS_CERT_FILE is required"},
		{"bad sample ratio", ServiceAPI, map[string]string{"ROUTER_TRACING_SAMPLE_RATIO": "2"}, "ROUTER_TRACING_SAMPLE_RATIO"},
		{"bad rate limit", ServiceAPI, map[string]string{"ROUTER_RATE_LIMIT_PER_MINUTE": "lots"}, "ROUTER_RATE_LIMIT_PER_MINUTE"},
	})
}

func TestLoaderDefaults(t *testing.T) {
	local, err := NewLoaderWithLookup(lookupFrom(nil)).Load(ServiceAPI)
	if err != nil {
		t.Fatal(err)
	}
	if !local.Auth.HasMode(AuthModeDev) || !local.Auth.HasMode(AuthModeOIDC) || !local.Auth.HasMode(AuthModeAPIToken) {
		t.Errorf("local modes = %v", local.Auth.Modes)
	}
	if local.Secrets.Provider != SecretsLocal || local.Auth.SessionIdleTTL != 8*time.Hour || local.Auth.SessionMaxTTL != 7*24*time.Hour {
		t.Errorf("unexpected local defaults: %+v", local)
	}
	if local.HTTP.CursorSecret == "" || local.Database.MigrateOnStart {
		t.Errorf("cursor secret %q migrate %v", local.HTTP.CursorSecret, local.Database.MigrateOnStart)
	}
	prod, err := NewLoaderWithLookup(lookupFrom(map[string]string{"ROUTER_ENV": "production", "ROUTER_CURSOR_SECRET": "c", "ROUTER_COOKIE_SECRET": "k"})).Load(ServiceAPI)
	if err != nil {
		t.Fatal(err)
	}
	if !prod.Auth.CookieSecure || local.Auth.CookieSecure {
		t.Errorf("cookie secure: prod %v local %v", prod.Auth.CookieSecure, local.Auth.CookieSecure)
	}
	if prod.Secrets.Provider != SecretsVault || prod.Internal.WorkloadAuth != WorkloadK8sToken || prod.Auth.HasMode(AuthModeDev) {
		t.Errorf("unexpected remote defaults: %+v", prod)
	}
	if local.RateLimit.Limiter != RateLimiterRedis || local.RateLimit.PerMinute != int(DefaultRateLimitPerMinute) || local.Tracing.Exporter != TracingNone {
		t.Errorf("rate limit %+v tracing %+v", local.RateLimit, local.Tracing)
	}
	if len(local.WebSocket.AllowedOrigins) != 1 || local.WebSocket.AllowedOrigins[0] != local.HTTP.PublicURL {
		t.Errorf("ws origins = %v", local.WebSocket.AllowedOrigins)
	}
	scopes, _ := NewLoaderWithLookup(lookupFrom(map[string]string{"ROUTER_OIDC_SCOPES": " openid , email ,"})).Load(ServiceAPI)
	if strings.Join(scopes.Auth.OIDC.Scopes, "|") != "openid|email" {
		t.Errorf("scopes = %v", scopes.Auth.OIDC.Scopes)
	}
}
