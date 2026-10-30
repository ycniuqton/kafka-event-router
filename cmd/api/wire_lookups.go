package main

import (
	"sync"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/library/resolver"
	"github.com/eventrouter/router/internal/lookups"
	lookupshttp "github.com/eventrouter/router/internal/lookups/http"
	"github.com/eventrouter/router/internal/lookups/pgstore"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/routes/assembly"
	"github.com/eventrouter/router/pkg/lookup"
)

// lookupsWiring composes the lookup-sources feature (connections 00001-lookup-sources) once per process.
type lookupsWiring struct {
	once     sync.Once
	public   api.Module
	internal api.Module
	service  *lookups.Service
	gateway  *lookup.Gateway
}

var lookupsFeature = &lookupsWiring{}

// lookupSourcesModule returns the public /api/v1/lookup-sources module.
func (s *apiService) lookupSourcesModule() api.Module {
	lookupsFeature.build(s.wiring)
	return lookupsFeature.public
}

// lookupSourcesInternalModule returns the workload-only spec module.
func (s *apiService) lookupSourcesInternalModule() api.Module {
	lookupsFeature.build(s.wiring)
	return lookupsFeature.internal
}

// lookupGateway is the in-process lookup gateway for API-side engine runs (DLQ dry-run/retry, test cases).
func (s *apiService) lookupGateway() *lookup.Gateway {
	return lookupsFeature.Gateway(s.wiring)
}

// Gateway returns the shared in-process gateway, composing the feature on first use.
func (l *lookupsWiring) Gateway(w *Wiring) *lookup.Gateway {
	l.build(w)
	return l.gateway
}

func (l *lookupsWiring) build(w *Wiring) {
	l.once.Do(func() {
		deps := w.Deps
		library := resolver.New(deps.DB)
		factories := lookup.DefaultFactories(library)
		l.service = lookups.NewService(lookups.ServiceDeps{Repo: pgstore.NewRepository(deps.DB, w.Tx), Usage: pgstore.NewUsageResolver(deps.DB),
			Vault: lookups.NewSecretVault(deps.Secrets), Validator: lookups.Validator{Kinds: lookups.DefaultKinds()}, Tx: w.Tx})
		l.gateway = lookup.NewGateway(lookup.GatewayDeps{Specs: l.service, Factories: factories})
		authz, err := w.Authorizers.Build(deps.Config.Auth.Authz)
		if err != nil {
			deps.Log.Error("lookup sources authorizer unavailable; team guard off", string(logging.AttrError), err)
		}
		d := lookupshttp.Deps{Service: l.service, Tests: lookups.TestService{Service: l.service, Tester: lookup.Tester{Factories: factories}},
			Responder: w.Responder, Pages: w.Pages, Recorder: deps.Audit, Authz: authz}
		l.public, l.internal = lookupshttp.NewModule(d), lookupshttp.NewInternalModule(d)
		w.registerLookupAuditActions()
	})
}

func init() {
	routesPortContributors = append(routesPortContributors, contributeLookupSourceRefs)
}

// contributeLookupSourceRefs adds the lookup-source reference rule to route validation (exists, same team, kind).
func contributeLookupSourceRefs(w *Wiring, p *assembly.Ports) {
	p.Validators = append(p.Validators, lookups.SourceRefValidator{Repo: pgstore.NewRepository(w.Deps.DB, w.Tx)})
}

// registerLookupAuditActions adds the lookup_source.* actions to access-control's catalog (secret replace is sensitive).
func (w *Wiring) registerLookupAuditActions() {
	if w.Access == nil || w.Access.Catalog == nil {
		return
	}
	for _, action := range lookups.Actions {
		w.Access.Catalog.Register(action, audit.ActionMeta{Category: audit.CategoryChange, Sensitive: action == lookups.ActionSecretReplaced})
	}
}
