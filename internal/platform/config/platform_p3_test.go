package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoaderP3Defaults(t *testing.T) {
	cfg, err := NewLoaderWithLookup(lookupFrom(map[string]string{})).Load(ServiceAPI)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Secrets.Rotation.Interval != 24*time.Hour || !cfg.Secrets.Rotation.Enabled() || cfg.Secrets.Rotation.PageSize != 100 ||
		len(cfg.Secrets.Rotation.LocalPreviousKeyFiles) != 0 {
		t.Fatalf("rotation defaults: %+v", cfg.Secrets.Rotation)
	}
	if cfg.Auth.OIDC.RoleClaims.Claim != "groups" || cfg.Auth.OIDC.RoleClaims.Enabled() {
		t.Fatalf("role claims defaults: %+v", cfg.Auth.OIDC.RoleClaims)
	}
	if len(cfg.AuditStream.Sinks) != 0 || cfg.AuditStream.Buffer != 1024 {
		t.Fatalf("audit stream defaults: %+v", cfg.AuditStream)
	}
}

func TestLoaderP3Values(t *testing.T) {
	cfg, err := NewLoaderWithLookup(lookupFrom(map[string]string{
		"ROUTER_SECRETS_LOCAL_PREVIOUS_KEY_FILES": "/a.key, /b.key", "ROUTER_SECRETS_ROTATION_INTERVAL": "0s",
		"ROUTER_OIDC_ROLE_CLAIM": "realm_access.roles", "ROUTER_OIDC_ROLE_MAPPINGS": "ops=Platform:admin,dev=Payments:editor",
		"ROUTER_AUDIT_STREAM_SINKS": "log", "ROUTER_AUDIT_STREAM_BUFFER": "8",
	})).Load(ServiceAPI)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Secrets.Rotation.Enabled() || strings.Join(cfg.Secrets.Rotation.LocalPreviousKeyFiles, "|") != "/a.key|/b.key" {
		t.Fatalf("rotation: %+v", cfg.Secrets.Rotation)
	}
	if cfg.Auth.OIDC.RoleClaims.Claim != "realm_access.roles" || len(cfg.Auth.OIDC.RoleClaims.Mappings) != 2 {
		t.Fatalf("role claims: %+v", cfg.Auth.OIDC.RoleClaims)
	}
	if len(cfg.AuditStream.Sinks) != 1 || cfg.AuditStream.Sinks[0] != AuditStreamLog || cfg.AuditStream.Buffer != 8 {
		t.Fatalf("audit stream: %+v", cfg.AuditStream)
	}
	if _, err := NewLoaderWithLookup(lookupFrom(map[string]string{"ROUTER_SECRETS_ROTATION_INTERVAL": "daily"})).Load(ServiceAPI); err == nil {
		t.Fatal("bad rotation interval accepted")
	}
}
