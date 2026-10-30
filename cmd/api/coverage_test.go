package main

import (
	alertshttp "github.com/eventrouter/router/internal/alerts/http"
	passwordhttp "github.com/eventrouter/router/internal/auth/password/httpapi"
	"io"
	"log/slog"
	"testing"

	approvalshttp "github.com/eventrouter/router/internal/approvals/http"
	assisthttp "github.com/eventrouter/router/internal/assist/http"
	"github.com/eventrouter/router/internal/audit"
	audithttp "github.com/eventrouter/router/internal/audit/http"
	"github.com/eventrouter/router/internal/auth"
	authhandlers "github.com/eventrouter/router/internal/auth/handlers"
	connhttp "github.com/eventrouter/router/internal/connections/http"
	dlqhttp "github.com/eventrouter/router/internal/dlq/httpapi"
	"github.com/eventrouter/router/internal/library/testcaseshttp"
	livetailhttp "github.com/eventrouter/router/internal/livetail/http"
	lookupshttp "github.com/eventrouter/router/internal/lookups/http"
	metricshttp "github.com/eventrouter/router/internal/metrics/http"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/preview/gateway"
	"github.com/eventrouter/router/internal/rbac"
	rbachttp "github.com/eventrouter/router/internal/rbac/http"
	routeshttp "github.com/eventrouter/router/internal/routes/http"
	"github.com/eventrouter/router/internal/runtime/controlapi"
)

// featureModules builds every public feature module with empty deps (route specs only, nothing is served).
func featureModules() []api.Module {
	return []api.Module{
		authhandlers.NewModule(authhandlers.Deps{Logins: auth.NewLoginManager(auth.LoginDeps{}), Config: config.Config{}}),
		rbachttp.NewModule(rbachttp.Deps{Passwords: &rbac.PasswordUserService{}}),
		passwordhttp.NewModule(passwordhttp.Deps{}),
		audithttp.NewModule(audithttp.Deps{Audit: audit.NewManager(nil, audit.NewCatalog())}),
		audithttp.NewExportModule(audithttp.Deps{Audit: audit.NewManager(nil, audit.NewCatalog())}),
		connhttp.NewModule(connhttp.Deps{}),
		lookupshttp.NewModule(lookupshttp.Deps{}),
		routeshttp.NewModule(routeshttp.Deps{}),
		dlqhttp.NewModule(dlqhttp.Deps{}),
		metricshttp.NewModule(metricshttp.Deps{}),
		controlapi.NewPublicModule(controlapi.PublicDeps{}),
		gateway.NewModule(gateway.Deps{}),
		assisthttp.NewModule(assisthttp.Deps{}),
		testcaseshttp.NewModule(testcaseshttp.Deps{}),
		approvalshttp.NewModule(approvalshttp.Deps{}),
		alertshttp.NewModule(alertshttp.Deps{}),
		livetailhttp.NewModule(api.NewWSUpgrader(api.WSOriginPolicy{}, 0, nil), livetailhttp.Deps{}),
	}
}

// TestEveryMountedRouteNamesACataloguedPermission fails when a feature route needs a permission missing from
// the rbac catalog (it would always be denied) or has none without being a known public route.
func TestEveryMountedRouteNamesACataloguedPermission(t *testing.T) {
	access := &AccessWiring{Coverage: rbac.NewCoverageChecker(rbac.DefaultPermissionCatalog(), PublicEndpoints.Keys()...)}
	report := access.VerifyCoverage(slog.New(slog.NewTextHandler(io.Discard, nil)), featureModules()...)
	if !report.OK() {
		t.Fatalf("unguarded=%v unknown=%v", report.Unguarded, report.Unknown)
	}
}
