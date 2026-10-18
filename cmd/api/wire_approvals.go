package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/internal/approvals"
	approvalshttp "github.com/eventrouter/router/internal/approvals/http"
	"github.com/eventrouter/router/internal/approvals/routesport"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/routes/assembly"
)

// ApprovalsComponents are the built approvals manager, its store and metrics.
type ApprovalsComponents struct {
	Manager *approvals.DefaultManager
	Store   *approvals.PostgresStore
	Metrics *approvals.Metrics
	Reader  approvals.DestinationReader
}

// approvalsBuilds memoises the approvals build per Wiring (routes ports and the HTTP module share it).
var approvalsBuilds sync.Map

// approvalsBuild is one memoised build result.
type approvalsBuild struct {
	once       sync.Once
	components *ApprovalsComponents
	err        error
}

// ApprovalsLog names log messages of the approvals wiring.
type ApprovalsLog string

const LogApprovalsDisabled ApprovalsLog = "approvals module disabled"

// ErrAccessNotBuilt is returned when approvals are built before access control.
var ErrAccessNotBuilt = errors.New("approvals: access control is not built yet")

func init() {
	routesPortContributors = append(routesPortContributors, contributeAccessControlPorts)
}

// contributeAccessControlPorts plugs team grants (validation), the policy-engine write guard and the deploy gate
// (approval policy) into routes.
func contributeAccessControlPorts(w *Wiring, ports *assembly.Ports) {
	if w.Access == nil {
		return
	}
	ports.Grants = rbac.NewGrantChecker(w.Access.Manager)
	ports.Guard = routesport.WriteGuard{Access: w.Access.Manager}
	c, err := w.ApprovalsComponents()
	if err != nil {
		w.Deps.Log.Error(string(LogApprovalsDisabled), string(logging.AttrError), err)
		return
	}
	ports.ApprovalPolicies = append(ports.ApprovalPolicies, routesport.GatePolicy{Gate: c.Manager, Destinations: c.Reader})
	ports.ApprovalRequests = RoutesApprovalRequests{Approvals: c.Manager}
}

// ApprovalsComponents builds (once) and returns the approvals components.
func (w *Wiring) ApprovalsComponents() (*ApprovalsComponents, error) {
	v, _ := approvalsBuilds.LoadOrStore(w, &approvalsBuild{})
	b := v.(*approvalsBuild)
	b.once.Do(func() { b.components, b.err = w.buildApprovals() })
	return b.components, b.err
}

// buildApprovals composes the approvals manager; activation resolves the routes components lazily.
func (w *Wiring) buildApprovals() (*ApprovalsComponents, error) {
	if w.Access == nil {
		return nil, ErrAccessNotBuilt
	}
	settings := approvals.NewSettingsLoader().Load()
	policy, err := approvals.NewPolicyRegistry().Build(settings.Policy)
	if err != nil {
		return nil, err
	}
	notifier, err := approvals.NewNotifierRegistry(w.Deps.Log).Build(settings.Notifier)
	if err != nil {
		return nil, err
	}
	st := approvals.NewPostgresStore(w.Deps.DB)
	reader := approvals.DestinationReader{Connections: w.Access.Conns}
	m := approvals.NewMetrics(nil, st)
	manager := approvals.NewManager(approvals.Deps{Tx: w.Tx, Store: st, Access: w.Access.Manager, Teams: w.Access.Teams,
		Destinations: reader, Policy: policy, Activator: &RoutesActivator{w: w}, Notifier: notifier, Trail: w.Access.Trail,
		Metrics: m, Now: time.Now, Log: w.Deps.Log})
	return &ApprovalsComponents{Manager: manager, Store: st, Metrics: m, Reader: reader}, nil
}

// RoutesActivator resolves routes' lifecycle on first use (routes is built after its ports, approvals among them).
type RoutesActivator struct {
	w *Wiring
}

// Revalidate implements approvals.Activator.
func (a *RoutesActivator) Revalidate(ctx context.Context, deploymentID uuid.UUID) error {
	port, err := a.port()
	if err != nil {
		return err
	}
	return port.Revalidate(ctx, deploymentID)
}

// Activate implements approvals.Activator.
func (a *RoutesActivator) Activate(ctx context.Context, deploymentID, approver uuid.UUID) error {
	port, err := a.port()
	if err != nil {
		return err
	}
	return port.Activate(ctx, deploymentID, approver)
}

func (a *RoutesActivator) port() (routesport.Activator, error) {
	c, err := a.w.RoutesComponents()
	if err != nil {
		return routesport.Activator{}, err
	}
	return routesport.Activator{Repos: c.Repos, Validator: c.Validator, Decider: c.ApprovalDecider()}, nil
}

// approvalsModule returns /api/v1/approvals and registers the approval metrics; a build error disables it (logged).
func (s *apiService) approvalsModule() api.Module {
	c, err := s.wiring.ApprovalsComponents()
	if err != nil {
		s.wiring.Deps.Log.Error(string(LogApprovalsDisabled), string(logging.AttrError), err)
		return api.Modules{}
	}
	for _, col := range c.Metrics.Collectors() {
		var dup prometheus.AlreadyRegisteredError
		if err := s.registry.Registerer().Register(col); err != nil && !errors.As(err, &dup) {
			s.wiring.Deps.Log.Error(string(LogApprovalsDisabled), string(logging.AttrError), err)
		}
	}
	return approvalshttp.NewModule(approvalshttp.Deps{Approvals: c.Manager, Owner: c.Store, Responder: s.wiring.Responder, Pages: s.wiring.Pages})
}
