package http

import (
	"testing"

	"github.com/eventrouter/router/internal/rbac"
)

func routeKeys(d Deps) map[string]bool {
	keys := map[string]bool{}
	for _, h := range NewModule(d).Handlers() {
		keys[h.Route().Method+" "+h.Route().Pattern] = true
	}
	return keys
}

func TestPasswordAdministrationMountedOnlyInPasswordMode(t *testing.T) {
	create, reset := "POST "+string(PathUsers), "POST "+string(PathUserPasswordReset)
	off := routeKeys(Deps{})
	if off[create] || off[reset] {
		t.Fatalf("password routes mounted without password mode: %v", off)
	}
	on := routeKeys(Deps{Passwords: &rbac.PasswordUserService{}})
	if !on[create] || !on[reset] {
		t.Fatalf("password routes missing in password mode")
	}
}

func TestPasswordAdministrationNeedsUserManage(t *testing.T) {
	for _, h := range NewModule(Deps{Passwords: &rbac.PasswordUserService{}}).Handlers() {
		switch h.(type) {
		case *CreateUserHandler, *ResetPasswordHandler:
			if h.Route().Permission != rbac.PermUserManage.Auth() || !h.Route().Mutating {
				t.Errorf("%s %s must require user.manage", h.Route().Method, h.Route().Pattern)
			}
		}
	}
}
