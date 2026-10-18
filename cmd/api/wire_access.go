package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/eventrouter/router/internal/audit"
	audithttp "github.com/eventrouter/router/internal/audit/http"
	"github.com/eventrouter/router/internal/auth"
	authhandlers "github.com/eventrouter/router/internal/auth/handlers"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/rbac"
	rbachttp "github.com/eventrouter/router/internal/rbac/http"
	"github.com/eventrouter/router/internal/rbac/opa"
)

// AccessWiring holds access control's composed components (other folders read Manager / Audit from it).
type AccessWiring struct {
	Manager  *rbac.DefaultManager
	Teams    *rbac.PostgresTeamStore
	Conns    *rbac.PostgresConnectionDirectory
	Trail    *rbac.AuditTrail
	Samples  rbac.SampleAccess
	Settings *rbac.SettingsService
	Audit    *audit.DefaultManager
	Catalog  *audit.Catalog
	Coverage *rbac.CoverageChecker
	Module   api.Module
}

// VerifyCoverage checks that every mounted route names a catalogued permission (fail closed) or is a
// known public route, and logs each gap; the engine denies unknown permissions at request time anyway.
func (a *AccessWiring) VerifyCoverage(log *slog.Logger, modules ...api.Module) rbac.CoverageReport {
	report := a.Coverage.Check(rbac.NewRuleTable(modules...))
	for _, key := range report.Unknown {
		log.Error(string(AccessLogUnknownPermission), string(AccessLogAttrRoute), key)
	}
	for _, key := range report.Unguarded {
		log.Error(string(AccessLogUnguarded), string(AccessLogAttrRoute), key)
	}
	return report
}

// AccessLogAttr names the log attributes of access wiring.
type AccessLogAttr string

const AccessLogAttrRoute AccessLogAttr = "route"

// AccessLog is a log message of access wiring.
type AccessLog string

const (
	AccessLogUnknownPermission AccessLog = "rbac: route requires a permission missing from the catalog (always denied)"
	AccessLogUnguarded         AccessLog = "rbac: route has no permission and is not a known public route"
)

// AccessAssembly builds access control: audit catalog + read side, policy engine, Manager,
// Authorizer registration, login Bootstrapper, /auth/me contributor, cache invalidation and the HTTP module.
type AccessAssembly struct {
	w *Wiring
}

// buildAccess is the wiring step for access control; it must run after buildAuth and before buildMounters.
func (w *Wiring) buildAccess(ctx context.Context) error {
	access, err := (&AccessAssembly{w: w}).Build(ctx)
	if err != nil {
		return err
	}
	w.Access = access
	return nil
}

// Build composes every access-control component.
func (a *AccessAssembly) Build(ctx context.Context) (*AccessWiring, error) {
	w, cfg := a.w, a.w.Deps.Config.Access
	catalog := a.auditCatalog()
	matrix, perms := rbac.DefaultRoleMatrix(), rbac.DefaultPermissionCatalog()
	engines := rbac.NewEngineRegistry()
	engines.Register(config.RBACEngineOPA, opa.Factory(opa.LoadSettings()))
	engine, err := engines.Build(cfg.Engine, rbac.EngineDeps{Matrix: matrix, Catalog: perms})
	if err != nil {
		return nil, err
	}
	assignable, err := rbac.ParseRoleSet(cfg.AssignableRoles)
	if err != nil {
		return nil, err
	}
	cache := rbac.NewTTLSubjectCache(cfg.CacheTTL, time.Now)
	loader := rbac.NewPostgresSubjectLoader(w.Deps.DB)
	conns, grants := rbac.NewPostgresConnectionDirectory(w.Deps.DB), rbac.NewPostgresGrantStore(w.Deps.DB)
	access := rbac.NewConnectionAccess(conns, grants)
	manager := rbac.NewManager(rbac.ManagerDeps{Engine: engine, Subjects: rbac.NewSubjectResolver(cache, loader),
		Matrix: matrix, Catalog: perms, Conns: access, Log: w.Deps.Log})
	w.Authorizers.Register(config.AuthzRBAC, func() (auth.Authorizer, error) { return rbac.NewAuthorizerAdapter(manager), nil })
	w.MeContributors = append(w.MeContributors, rbac.NewAccessContributor(manager))
	admins := rbac.NewPostgresAdminStore(w.Deps.DB)
	w.Logins.SetHook(a.bootstrapper(admins))
	trail := rbac.NewAuditTrail(w.Deps.Audit)
	go rbac.NewInvalidationListener(w.Deps.DB.Pool(), cache, w.Deps.Log).Run(ctx)
	reader := audit.NewManager(audit.NewPostgresReader(w.Deps.DB), catalog)
	teams := rbac.NewPostgresTeamStore(w.Deps.DB)
	settings := rbac.NewSettingsService(rbac.SettingsServiceDeps{Tx: w.Tx, Store: rbac.NewPostgresSettingsStore(w.Deps.DB),
		Catalog: rbac.DefaultSettingCatalog(), Trail: trail})
	parts := AccessParts{Loader: loader, Admins: admins, Assignable: assignable, Teams: teams, Grants: grants, Access: access, Trail: trail,
		Settings: settings}
	return &AccessWiring{Manager: manager, Samples: rbac.NewSampleAccess(manager), Teams: teams, Conns: conns, Trail: trail,
		Settings: settings, Catalog: catalog, Audit: reader,
		Coverage: rbac.NewCoverageChecker(perms, PublicEndpoints.Keys()...),
		Module: api.Modules(append(a.module(parts).Handlers(),
			audithttp.NewModule(audithttp.Deps{Audit: reader, Responder: w.Responder, Pages: w.Pages}).Handlers()...))}, nil
}

// auditCatalog plugs the action catalog into the recorder (unknown actions fail, sensitivity is stamped).
func (a *AccessAssembly) auditCatalog() *audit.Catalog {
	catalog := audit.NewCatalog()
	if hooks, ok := a.w.Deps.Audit.(audit.HookRegistrar); ok {
		hooks.AddHook(audit.NewCatalogHook(catalog))
	}
	return catalog
}

func (a *AccessAssembly) bootstrapper(admins rbac.AdminStore) *rbac.Bootstrapper {
	w := a.w
	return rbac.NewBootstrapper(rbac.BootstrapDeps{Invites: rbac.NewPostgresInvitationStore(w.Deps.DB),
		Rules: []rbac.BootstrapRule{rbac.DevSeedRule{}, rbac.NewEmailAllowListRule(w.Deps.Config.Access.BootstrapAdminEmails),
			rbac.NewFirstAdminRule(admins)},
		Admins: admins, Teams: w.Identity.Teams, Bindings: w.Identity.RoleBindings,
		TeamName: w.Deps.Config.Auth.Dev.TeamName, Recorder: w.Deps.Audit,
	})
}

// AccessParts are the stores and rules shared by the access-control services.
type AccessParts struct {
	Loader     rbac.SubjectLoader
	Admins     rbac.AdminStore
	Assignable rbac.RoleSet
	Teams      *rbac.PostgresTeamStore
	Grants     *rbac.PostgresGrantStore
	Access     *rbac.ConnectionAccess
	Trail      *rbac.AuditTrail
	Settings   *rbac.SettingsService
}

func (a *AccessAssembly) module(p AccessParts) api.Module {
	w := a.w
	guard, userStore, bindingStore := rbac.NewAdminGuard(p.Admins), rbac.NewPostgresUserStore(w.Deps.DB), rbac.NewPostgresBindingStore(w.Deps.DB)
	users := rbac.NewUserService(rbac.UserServiceDeps{Tx: w.Tx, Users: userStore, Guard: guard, Sessions: w.Sessions, Trail: p.Trail})
	bindings := rbac.NewBindingService(rbac.BindingServiceDeps{Tx: w.Tx, Bindings: bindingStore,
		Subjects: p.Loader, Guard: guard, Assignable: p.Assignable, Trail: p.Trail})
	teams := rbac.NewTeamService(rbac.TeamServiceDeps{Tx: w.Tx, Teams: p.Teams, Trail: p.Trail, Defaults: p.Settings})
	grants := rbac.NewGrantService(rbac.GrantServiceDeps{Tx: w.Tx, Grants: p.Grants, Access: p.Access, Teams: p.Teams, Trail: p.Trail})
	inviter := rbac.NewInviter(rbac.InviterDeps{Tx: w.Tx, Invites: rbac.NewPostgresInvitationStore(w.Deps.DB), Bindings: bindingStore,
		Teams: p.Teams, Users: userStore, Assignable: p.Assignable, Trail: p.Trail})
	return rbachttp.NewModule(rbachttp.Deps{Users: users, Bindings: bindings, Teams: teams, Grants: grants, Inviter: inviter,
		Settings: p.Settings, Responder: w.Responder, Pages: w.Pages})
}

// MeContributors is the list of /auth/me contributors gathered during wiring.
type MeContributors = []authhandlers.MeContributor

// EndpointList is a set of "METHOD /pattern" route keys.
type EndpointList []string

// Keys returns the route keys.
func (l EndpointList) Keys() []string { return l }

// PublicEndpoints are the routes served without a route-level permission: the login flow, the caller's own
// session, and collections that filter each object by permission themselves (global DLQ list/summary: dlq.read).
var PublicEndpoints = EndpointList{
	"GET /auth/config", "GET /auth/oidc/login", "GET /auth/oidc/callback", "POST /auth/dev/login", "POST /auth/logout",
	"GET /api/v1/auth/me", "GET /api/v1/dlq", "GET /api/v1/dlq/summary",
	"GET /api/v1/auth/tokens", "POST /api/v1/auth/tokens", "DELETE /api/v1/auth/tokens/{id}",
}
