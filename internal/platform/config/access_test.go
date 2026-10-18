package config

import (
	"testing"
	"time"
)

func TestAccessConfigDefaultsAndOverrides(t *testing.T) {
	cfg, err := NewLoaderWithLookup(func(string) (string, bool) { return "", false }).Load(ServiceAPI)
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Access
	if a.Engine != RBACEngineRoleBinding || a.CacheTTL != 30*time.Second || len(a.AssignableRoles) != 4 || len(a.BootstrapAdminEmails) != 0 {
		t.Fatalf("defaults = %+v", a)
	}
	if cfg.Auth.Authz != AuthzRBAC {
		t.Fatalf("authz default = %s", cfg.Auth.Authz)
	}
	env := map[string]string{string(EnvRBACCacheTTL): "5s", string(EnvBootstrapAdminEmails): "a@x.io, b@x.io", string(EnvRBACAssignableRoles): "admin,editor,viewer,approver"}
	cfg, err = NewLoaderWithLookup(func(k string) (string, bool) { v, ok := env[k]; return v, ok }).Load(ServiceAPI)
	if err != nil || cfg.Access.CacheTTL != 5*time.Second || len(cfg.Access.BootstrapAdminEmails) != 2 || len(cfg.Access.AssignableRoles) != 4 {
		t.Fatalf("overrides = %+v %v", cfg.Access, err)
	}
	env[string(EnvRBACCacheTTL)] = "soon"
	if _, err := NewLoaderWithLookup(func(k string) (string, bool) { v, ok := env[k]; return v, ok }).Load(ServiceAPI); err == nil {
		t.Fatal("bad duration accepted")
	}
}
