package assembly

import (
	"context"
	"log/slog"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/bulk"
	routeshttp "github.com/eventrouter/router/internal/routes/http"
	"github.com/eventrouter/router/internal/routes/portability"
	"github.com/eventrouter/router/internal/routes/validation"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/store/routestore"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/config/validators"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
)

// Ports are optional collaborators owned by other folders; nil ports disable their validators.
type Ports struct {
	Topics           validation.TopicInspector
	SampleSchemas    validation.SampleSchemaProvider
	Grants           validation.GrantChecker
	MetricsReaders   []routes.RouteMetricsReader
	ApprovalPolicies []routes.ApprovalPolicy
	ApprovalRequests routes.ApprovalRequester
	Guard            routes.WriteGuard
	TestCases        validation.TestCaseRunner
	LibraryResolver  config.LibraryResolver
	Rollouts         []routes.RolloutNotifier
	Library          validation.LibraryLookup
	Replays          routes.OffsetResetter
	// Validators are extra external-data rules contributed by other folders (e.g. lookup-source refs).
	Validators []validation.ContextValidator
}

// Deps is the shared infrastructure the feature is built from.
type Deps struct {
	DB        *store.DB
	Tx        store.TxManager
	Notifier  *store.Notifier
	Recorder  audit.Recorder
	Responder *api.Responder
	Pages     *api.PageParser
	Log       *slog.Logger
	Settings  Settings
	Ports     Ports
}

// Components are the built managers other folders call in-process, plus the HTTP module.
type Components struct {
	Routes      *routes.RouteManager
	State       *routes.StateManager
	Deploy      *routes.DeployManager
	Versions    *routes.VersionManager
	Validate    *routes.ValidateManager
	Lifecycle   *routes.DeploymentLifecycle
	Validator   *validation.ValidationManager
	Repos       routes.Repositories
	Scripts     ScriptChecker
	Portability *portability.PortabilityManager
	Replays     *routes.ReplayManager
	Module      api.Module
}

// RolloutReporter exposes the lifecycle to route-runtime.
func (c *Components) RolloutReporter() routes.RolloutReporter { return c.Lifecycle }

// ApprovalDecider exposes the lifecycle to access-control approvals.
func (c *Components) ApprovalDecider() routes.ApprovalDecider { return c.Lifecycle }

// Build composes the feature.
func Build(ctx context.Context, d Deps) (*Components, error) {
	routeRepo := routestore.NewRouteRepository(d.DB)
	repos := routes.Repositories{Routes: routeRepo, Versions: routestore.NewVersionRepository(d.DB), Deployments: routestore.NewDeploymentRepository(d.DB)}
	connections := routestore.NewConnectionLookup(d.DB)
	scripts, err := ScriptCheckerSelector{Log: d.Log}.Select(ctx, d.Settings.ScriptCheck)
	if err != nil {
		return nil, err
	}
	validator, err := ValidatorFactory{Ports: d.Ports, Connections: connections, Names: routeRepo, Scripts: scripts}.Build(d.Settings)
	if err != nil {
		return nil, err
	}
	core, approval, err := CoreFactory{Deps: d, Repos: repos, Connections: connections, Validator: validator}.Build()
	if err != nil {
		return nil, err
	}
	c := &Components{Routes: routes.NewRouteManager(core), State: routes.NewStateManager(core), Deploy: routes.NewDeployManager(core, approval),
		Versions: routes.NewVersionManager(core), Validate: routes.NewValidateManager(core), Lifecycle: routes.NewDeploymentLifecycle(core),
		Validator: validator, Repos: repos, Scripts: scripts}
	c.Deploy.Requests = d.Ports.ApprovalRequests
	if c.Lifecycle.Notices, err = routes.NewRolloutNotifierManager(d.Settings.RolloutNotifier, d.Log, d.Ports.Rollouts...); err != nil {
		return nil, err
	}
	if c.Portability, err = (PortabilityFactory{Core: core, Components: c, Directory: connections}).Build(); err != nil {
		return nil, err
	}
	c.Replays = routes.NewReplayManager(core, d.Ports.Replays, routes.ReplayRules{Window: d.Settings.ReplayWindow})
	c.Module = routeshttp.NewModule(routeshttp.Deps{Routes: c.Routes, State: c.State, Deploy: c.Deploy, Versions: c.Versions,
		Validate: c.Validate, Bulk: bulk.NewBulkActionManager(problem.NewCatalog(), bulk.PauseHandler{State: c.State},
			bulk.ResumeHandler{State: c.State}, bulk.AddTagHandler{Routes: c.Routes}, bulk.ExportHandler{Portability: c.Portability}),
		Portability: c.Portability, Replays: c.Replays, Responder: d.Responder, Pages: d.Pages})
	return c, nil
}

// PortabilityFactory builds export/import over the built managers.
type PortabilityFactory struct {
	Core       *routes.Core
	Components *Components
	Directory  portability.ConnectionDirectory
}

// Build returns the portability manager with the default YAML/JSON codecs.
func (f PortabilityFactory) Build() (*portability.PortabilityManager, error) {
	codecs, err := portability.DefaultCodecs()
	if err != nil {
		return nil, err
	}
	c := f.Components
	return portability.NewPortabilityManager(portability.Deps{Routes: c.Routes, Versions: c.Versions, Repos: c.Repos, Validator: c.Validator,
		Resolver: portability.ConnectionResolver{Directory: f.Directory}, Codecs: codecs, Auditor: f.Core.Auditor}), nil
}

// ScriptCheckerSelector builds the configured script checker, falling back to skip when the sandbox cannot start.
type ScriptCheckerSelector struct {
	Log *slog.Logger
}

// Select returns the checker of kind.
func (s ScriptCheckerSelector) Select(ctx context.Context, kind ScriptCheckKind) (ScriptChecker, error) {
	factory, ok := ScriptCheckers[kind]
	if !ok {
		return nil, ErrUnknownScriptCheck{Kind: kind}
	}
	checker, err := factory(ctx)
	if err != nil {
		s.Log.Warn("routes: JS compile checks disabled", string(logging.AttrError), err)
		return SkipScriptChecker{}, nil
	}
	return checker, nil
}

// ErrUnknownScriptCheck reports an unsupported ROUTER_ROUTES_SCRIPT_CHECK value.
type ErrUnknownScriptCheck struct {
	Kind ScriptCheckKind
}

func (e ErrUnknownScriptCheck) Error() string {
	return "routes: unknown " + string(EnvScriptCheck) + " " + string(e.Kind)
}

// ValidatorFactory registers the doc-only validator and every external-data validator whose port exists.
type ValidatorFactory struct {
	Ports       Ports
	Connections validation.ConnectionLookup
	Names       validation.RouteNameIndex
	Scripts     ScriptChecker
}

// Build returns the validation manager.
func (f ValidatorFactory) Build(s Settings) (*validation.ValidationManager, error) {
	docs, err := validators.NewDefaultManager(validators.Deps{Compiler: compile.NewDefaultCompiler(), Options: engine.CompileOptions{Scripts: f.Scripts.Runtime(), Library: f.Ports.LibraryResolver}})
	if err != nil {
		return nil, err
	}
	set := validation.NewValidatorSet(validation.DocumentValidator{Checker: docs},
		validation.ConnectionAccessValidator{Lookup: f.Connections, Grants: f.Ports.Grants},
		validation.UniqueNameValidator{Index: f.Names}, validation.SchemaRegistryValidator{Lookup: f.Connections},
		validation.SourceStartValidator{}, validation.ExactlyOnceValidator{})
	if f.Ports.Topics != nil {
		set.Add(validation.DestinationTopicValidator{Inspector: f.Ports.Topics})
	}
	if f.Ports.SampleSchemas != nil {
		set.Add(validation.FieldTypeValidator{Schemas: f.Ports.SampleSchemas})
	}
	if f.Ports.TestCases != nil {
		set.Add(validation.TestCaseValidator{Runner: f.Ports.TestCases})
	}
	if f.Ports.Library != nil {
		set.Add(validation.LibraryRefValidator{Library: f.Ports.Library})
	}
	for _, v := range f.Ports.Validators {
		set.Add(v)
	}
	return validation.NewValidationManager(set, validation.ManagerOptions{Parallelism: s.ValidationParallel, Timeout: s.ValidationTimeout}), nil
}

// CoreFactory builds the shared manager core and the approval policy manager.
type CoreFactory struct {
	Deps        Deps
	Repos       routes.Repositories
	Connections validation.ConnectionLookup
	Validator   *validation.ValidationManager
}

// Build selects the metrics reader and approval policy from settings.
func (f CoreFactory) Build() (*routes.Core, *routes.ApprovalPolicyManager, error) {
	d := f.Deps
	readers := routes.NewMetricsReaderRegistry()
	for _, r := range d.Ports.MetricsReaders {
		readers.Register(r)
	}
	metrics, err := readers.Select(d.Settings.MetricsReader)
	if err != nil {
		return nil, nil, err
	}
	approval, err := routes.NewApprovalPolicyManager(d.Settings.ApprovalPolicy, d.Ports.ApprovalPolicies...)
	if err != nil {
		return nil, nil, err
	}
	guard := d.Ports.Guard
	if guard == nil {
		guard = routes.TeamRoleWriteGuard{}
	}
	core := &routes.Core{Repos: f.Repos, Tx: d.Tx, Validator: f.Validator, Auditor: routes.RouteAuditor{Fallback: d.Recorder},
		Views:  routes.SummaryBuilder{Connections: f.Connections, Metrics: metrics, Log: d.Log},
		Events: routestore.PgNotifyPublisher{Notifier: d.Notifier}, Guard: guard, Log: d.Log}
	return core, approval, nil
}
