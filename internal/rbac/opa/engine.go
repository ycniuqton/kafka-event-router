// Package opa is the Open Policy Agent implementation of rbac.PolicyEngine (ROUTER_RBAC_ENGINE=opa):
// an embedded Rego policy evaluated in-process over the built-in role matrix exported as OPA data.
package opa

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage/inmem"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/rbac"
)

//go:embed policy.rego
var builtinPolicy string

// Query names the Rego rules the engine evaluates.
type Query string

const (
	QueryDecision    Query = "data.router.authz.decision"
	QueryPermissions Query = "data.router.authz.permissions"
	ModuleName       Query = "router_authz.rego"
)

// EnvKey names the engine's environment keys (read by cmd/api only).
type EnvKey string

// EnvPolicyFile replaces the built-in policy with a Rego file (same package and rules).
const EnvPolicyFile EnvKey = "ROUTER_RBAC_OPA_POLICY"

// ErrBadResult is returned when the policy answers with an unexpected shape (the call is denied).
var ErrBadResult = errors.New("opa: policy returned an unexpected result")

// Engine evaluates prepared Rego queries; every evaluation error fails closed (engine_error).
type Engine struct {
	decision    rego.PreparedEvalQuery
	permissions rego.PreparedEvalQuery
}

// Settings selects the policy source.
type Settings struct {
	PolicyFile string
}

// LoadSettings reads the settings from the environment.
func LoadSettings() Settings {
	return Settings{PolicyFile: os.Getenv(string(EnvPolicyFile))}
}

// Factory returns the rbac.EngineFactory registered as kind "opa".
func Factory(s Settings) rbac.EngineFactory {
	return func(d rbac.EngineDeps) (rbac.PolicyEngine, error) {
		policy := builtinPolicy
		if s.PolicyFile != "" {
			raw, err := os.ReadFile(s.PolicyFile)
			if err != nil {
				return nil, fmt.Errorf("opa: read %s: %w", EnvPolicyFile, err)
			}
			policy = string(raw)
		}
		return New(context.Background(), d, policy)
	}
}

// New prepares both queries over the policy and the matrix data.
func New(ctx context.Context, d rbac.EngineDeps, policy string) (*Engine, error) {
	store := inmem.NewFromObject(DataDocument{}.Of(d.Matrix, d.Catalog))
	prepare := func(q Query) (rego.PreparedEvalQuery, error) {
		return rego.New(rego.Query(string(q)), rego.Module(string(ModuleName), policy), rego.Store(store)).PrepareForEval(ctx)
	}
	decision, err := prepare(QueryDecision)
	if err != nil {
		return nil, fmt.Errorf("opa: prepare decision: %w", err)
	}
	permissions, err := prepare(QueryPermissions)
	if err != nil {
		return nil, fmt.Errorf("opa: prepare permissions: %w", err)
	}
	return &Engine{decision: decision, permissions: permissions}, nil
}

// Kind implements rbac.PolicyEngine.
func (e *Engine) Kind() rbac.EngineKind { return config.RBACEngineOPA }

// Evaluate implements rbac.PolicyEngine.
func (e *Engine) Evaluate(ctx context.Context, s rbac.Subject, p rbac.Permission, r rbac.Resource) rbac.Decision {
	rs, err := e.decision.Eval(ctx, rego.EvalInput(Input{}.Decision(s, p, r)))
	if err != nil {
		return rbac.Deny(rbac.ReasonEngineError, p)
	}
	out, err := ResultReader{}.Decision(rs)
	if err != nil {
		return rbac.Deny(rbac.ReasonEngineError, p)
	}
	if out.Allowed {
		return rbac.Allow()
	}
	return rbac.Deny(out.Reason, p)
}

// Permissions implements rbac.PolicyEngine; an evaluation error yields no permissions (fail closed).
func (e *Engine) Permissions(s rbac.Subject, team rbac.TeamID) rbac.PermissionSet {
	rs, err := e.permissions.Eval(context.Background(), rego.EvalInput(Input{}.Permissions(s, team)))
	if err != nil {
		return rbac.NewPermissionSet()
	}
	perms, err := ResultReader{}.Permissions(rs)
	if err != nil {
		return rbac.NewPermissionSet()
	}
	return rbac.NewPermissionSet(perms...)
}

// Input renders policy inputs: subject {disabled, workspaceAdmin, bindings{teamId: role}}, permission, resource, team.
type Input struct{}

// Decision is the input of the decision query.
func (Input) Decision(s rbac.Subject, p rbac.Permission, r rbac.Resource) map[string]any {
	return map[string]any{string(FieldSubject): Input{}.subject(s), string(FieldPermission): string(p),
		string(FieldResource): map[string]any{string(FieldType): string(r.Type), string(FieldID): r.ID,
			string(FieldTeamID): Input{}.team(r.TeamID)}}
}

// Permissions is the input of the permissions query.
func (Input) Permissions(s rbac.Subject, team rbac.TeamID) map[string]any {
	return map[string]any{string(FieldSubject): Input{}.subject(s), string(FieldTeam): Input{}.team(team)}
}

func (Input) subject(s rbac.Subject) map[string]any {
	bindings := make(map[string]any, len(s.Bindings))
	for team, role := range s.Bindings {
		bindings[team.String()] = string(role)
	}
	return map[string]any{string(FieldDisabled): s.Disabled, string(FieldWorkspaceAdmin): s.WorkspaceAdmin,
		string(FieldBindings): bindings}
}

func (Input) team(t rbac.TeamID) string {
	if t == uuid.Nil {
		return ""
	}
	return t.String()
}

// DecisionResult is the decoded decision document.
type DecisionResult struct {
	Allowed bool
	Reason  rbac.DenyReason
}

// ResultReader decodes query results.
type ResultReader struct{}

// Decision decodes {"allowed": bool, "reason": string}.
func (ResultReader) Decision(rs rego.ResultSet) (DecisionResult, error) {
	if len(rs) != 1 || len(rs[0].Expressions) != 1 {
		return DecisionResult{}, ErrBadResult
	}
	doc, ok := rs[0].Expressions[0].Value.(map[string]any)
	if !ok {
		return DecisionResult{}, ErrBadResult
	}
	allowed, okAllowed := doc[string(FieldAllowed)].(bool)
	reason, okReason := doc[string(FieldReason)].(string)
	if !okAllowed || !okReason {
		return DecisionResult{}, ErrBadResult
	}
	return DecisionResult{Allowed: allowed, Reason: rbac.DenyReason(reason)}, nil
}

// Permissions decodes a set of permission names.
func (ResultReader) Permissions(rs rego.ResultSet) ([]rbac.Permission, error) {
	if len(rs) != 1 || len(rs[0].Expressions) != 1 {
		return nil, ErrBadResult
	}
	items, ok := rs[0].Expressions[0].Value.([]any)
	if !ok {
		return nil, ErrBadResult
	}
	out := make([]rbac.Permission, 0, len(items))
	for _, item := range items {
		name, ok := item.(string)
		if !ok {
			return nil, ErrBadResult
		}
		out = append(out, rbac.Permission(name))
	}
	return out, nil
}

// Field names members of policy documents.
type Field string

const (
	FieldAllowed        Field = "allowed"
	FieldReason         Field = "reason"
	FieldRouter         Field = "router"
	FieldPermissions    Field = "permissions"
	FieldRoles          Field = "roles"
	FieldScope          Field = "scope"
	FieldCrossTeam      Field = "crossTeam"
	FieldSubject        Field = "subject"
	FieldPermission     Field = "permission"
	FieldResource       Field = "resource"
	FieldType           Field = "type"
	FieldID             Field = "id"
	FieldTeamID         Field = "teamId"
	FieldTeam           Field = "team"
	FieldDisabled       Field = "disabled"
	FieldBindings       Field = "bindings"
	FieldWorkspaceAdmin Field = "workspaceAdmin"
)

// DataDocument exports the role matrix and permission catalog as OPA data (data.router.*).
type DataDocument struct{}

// Of returns {"router": {"permissions": {perm: {scope, crossTeam}}, "roles": {role: [perm…]}}}.
func (DataDocument) Of(m rbac.RoleMatrix, c rbac.PermissionCatalog) map[string]any {
	perms := make(map[string]any, len(c))
	for p, meta := range c {
		perms[string(p)] = map[string]any{string(FieldScope): string(meta.Scope), string(FieldCrossTeam): meta.CrossTeam}
	}
	roles := make(map[string]any, len(m))
	for role, set := range m {
		names := make([]any, 0, len(set))
		for _, p := range set.Sorted() {
			names = append(names, string(p))
		}
		roles[string(role)] = names
	}
	return map[string]any{string(FieldRouter): map[string]any{string(FieldPermissions): perms, string(FieldRoles): roles}}
}
