package rbac

import (
	"context"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
)

// EndpointRule is the permission one mounted route requires (taken from its RouteSpec).
type EndpointRule struct {
	Method     string
	Pattern    string
	Permission Permission
	Public     bool
}

// Key identifies the rule's route.
func (r EndpointRule) Key() string {
	return r.Method + " " + r.Pattern
}

// RuleTable is the registry of endpoint rules built from mounted modules (coverage tests, docs).
type RuleTable struct {
	rules map[string]EndpointRule
}

// NewRuleTable builds the table from every handler of modules.
func NewRuleTable(modules ...api.Module) *RuleTable {
	t := &RuleTable{rules: map[string]EndpointRule{}}
	for _, mod := range modules {
		for _, h := range mod.Handlers() {
			t.Register(RuleOf(h.Route()))
		}
	}
	return t
}

// RuleOf derives the rule of one route spec; routes without a permission are public or auth-only.
func RuleOf(spec api.RouteSpec) EndpointRule {
	return EndpointRule{Method: spec.Method, Pattern: spec.Pattern, Permission: Permission(spec.Permission.Key),
		Public: spec.Permission.IsZero()}
}

// Register adds or replaces a rule.
func (t *RuleTable) Register(r EndpointRule) {
	t.rules[r.Key()] = r
}

// Lookup returns the rule of a route.
func (t *RuleTable) Lookup(method, pattern string) (EndpointRule, bool) {
	r, ok := t.rules[method+" "+pattern]
	return r, ok
}

// Rules returns every rule ordered by key.
func (t *RuleTable) Rules() []EndpointRule {
	out := make([]EndpointRule, 0, len(t.rules))
	for _, r := range t.rules {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

// CoverageReport lists routes that would be denied or left unguarded.
type CoverageReport struct {
	Unguarded []string
	Unknown   []string
}

// OK reports whether every route is guarded by a known permission or explicitly public.
func (c CoverageReport) OK() bool {
	return len(c.Unguarded) == 0 && len(c.Unknown) == 0
}

// CoverageChecker verifies that each route names a catalogued permission (fail closed) or is allow-listed.
type CoverageChecker struct {
	catalog PermissionCatalog
	public  map[string]bool
}

// NewCoverageChecker builds a checker; publicKeys are "METHOD /pattern" routes allowed without permission.
func NewCoverageChecker(catalog PermissionCatalog, publicKeys ...string) *CoverageChecker {
	c := &CoverageChecker{catalog: catalog, public: map[string]bool{}}
	for _, k := range publicKeys {
		c.public[k] = true
	}
	return c
}

// Check inspects every rule of the table.
func (c *CoverageChecker) Check(t *RuleTable) CoverageReport {
	var rep CoverageReport
	for _, r := range t.Rules() {
		if r.Public && !c.public[r.Key()] {
			rep.Unguarded = append(rep.Unguarded, r.Key())
		}
		if _, known := c.catalog[r.Permission]; !r.Public && !known {
			rep.Unknown = append(rep.Unknown, r.Key())
		}
	}
	return rep
}

// TeamOwner resolves the owning team of an object id (implemented by feature repositories).
type TeamOwner interface {
	TeamOf(ctx context.Context, id string) (string, error)
}

// ObjectResolvers builds auth.ObjectResolver values for RouteSpec.Object.
type ObjectResolvers struct{}

// PathTeam targets the team named by a path parameter.
func (ObjectResolvers) PathTeam(param string) auth.ObjectResolver {
	return func(r *http.Request) auth.ObjectRef {
		return auth.ObjectRef{Type: string(ResourceTeam), TeamID: chi.URLParam(r, param)}
	}
}

// Owned targets an object whose team is looked up by id; a failed lookup leaves the team unknown.
func (ObjectResolvers) Owned(t ResourceType, param string, owner TeamOwner) auth.ObjectResolver {
	return func(r *http.Request) auth.ObjectRef {
		id := chi.URLParam(r, param)
		team, err := owner.TeamOf(r.Context(), id)
		if err != nil {
			team = ""
		}
		return auth.ObjectRef{Type: string(t), ID: id, TeamID: team}
	}
}
