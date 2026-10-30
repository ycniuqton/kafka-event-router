package audit

import (
	"context"
	"errors"
	"sort"
	"sync"
)

// Catalogued actions owned by access-control (design §2.3); each feature records only actions listed here.
const (
	ActionRouteCreated         Action = "route.created"
	ActionRouteDraftSaved      Action = "route.draft_saved"
	ActionRouteDeployed        Action = "route.deployed"
	ActionRouteDeployRequested Action = "route.deploy_requested"
	ActionRoutePaused          Action = "route.paused"
	ActionRouteResumed         Action = "route.resumed"
	ActionRouteRolledBack      Action = "route.rolled_back"
	ActionRouteArchived        Action = "route.archived"
	ActionRouteImported        Action = "route.imported"
	ActionRouteReplayed        Action = "route.replayed"
	ActionConnectionCreated    Action = "connection.created"
	ActionConnectionUpdated    Action = "connection.updated"
	ActionConnectionDeleted    Action = "connection.deleted"
	ActionConnectionSecretSet  Action = "connection.secret_replaced"
	ActionConnectionTested     Action = "connection.tested"
	ActionTopicCreated         Action = "topic.created"
	ActionTransformSaved       Action = "transform.saved"
	ActionLibraryPublished     Action = "library.published"
	ActionTestCaseSaved        Action = "testcase.saved"
	ActionAlertRuleSaved       Action = "alert_rule.saved"
	ActionDeploymentApproved   Action = "deployment.approved"
	ActionDeploymentRejected   Action = "deployment.rejected"
	ActionDeploymentWithdrawn  Action = "deployment.withdrawn"
	ActionApprovalCommented    Action = "approval.commented"
	ActionRoleBindingSet       Action = "role_binding.set"
	ActionRoleBindingRemoved   Action = "role_binding.removed"
	ActionUserUpdated          Action = "user.updated"
	ActionTeamCreated          Action = "team.created"
	ActionTeamUpdated          Action = "team.updated"
	ActionTeamDeleted          Action = "team.deleted"
	ActionGrantAdded           Action = "grant.added"
	ActionGrantRemoved         Action = "grant.removed"
	ActionSettingsUpdated      Action = "settings.updated"
	ActionSampleViewed         Action = "sample.viewed"
	ActionSampleViewedRaw      Action = "sample.viewed_raw"
	ActionLiveTailOpened       Action = "livetail.opened"
	ActionDLQViewed            Action = "dlq.viewed"
	ActionDLQRetried           Action = "dlq.retried"
	ActionDLQDiscarded         Action = "dlq.discarded"
	ActionDLQExported          Action = "dlq.exported"
	ActionAuditExported        Action = "audit.exported"
	ActionUserInvited          Action = "user.invited"
	ActionInvitationAccepted   Action = "user.invitation_accepted"
)

// Catalogued object types added by access-control on top of the write side's.
const (
	ObjectTopic         ObjectType = "topic"
	ObjectSettings      ObjectType = "settings"
	ObjectSampleSession ObjectType = "sample_session"
	ObjectDLQ           ObjectType = "dlq"
	ObjectAuditLog      ObjectType = "audit_log"
	ObjectGrant         ObjectType = "grant"
)

// Category groups actions for the audit screen: changes, data access, administration.
type Category string

const (
	CategoryChange Category = "change"
	CategoryAccess Category = "access"
	CategoryAdmin  Category = "admin"
)

// ActionMeta describes one catalogued action; Sensitive entries are highlighted and filterable.
type ActionMeta struct {
	Sensitive bool     `json:"sensitive"`
	Category  Category `json:"category"`
}

// ErrUnknownAction is returned by the recorder hook for actions missing from the catalog.
var ErrUnknownAction = errors.New("action is not in the audit catalog")

// Catalog is the single source of audited actions; features may Register extra ones at wiring time.
type Catalog struct {
	mu      sync.RWMutex
	actions map[Action]ActionMeta
}

// NewCatalog returns the catalog pre-loaded with every action of design §2.3 plus platform-core's.
func NewCatalog() *Catalog {
	c := &Catalog{actions: map[Action]ActionMeta{}}
	c.registerAll(CategoryChange, false, ActionRouteCreated, ActionRouteDraftSaved, ActionRouteDeployed,
		ActionRouteDeployRequested, ActionRoutePaused, ActionRouteResumed, ActionRouteRolledBack, ActionRouteArchived,
		ActionRouteImported, ActionRouteReplayed, ActionConnectionCreated, ActionConnectionUpdated, ActionConnectionDeleted,
		ActionConnectionTested, ActionTopicCreated, ActionTransformSaved, ActionLibraryPublished, ActionTestCaseSaved,
		ActionAlertRuleSaved, ActionDeploymentApproved, ActionDeploymentRejected, ActionDeploymentWithdrawn,
		ActionApprovalCommented, ActionDLQRetried, ActionDLQDiscarded)
	c.registerAll(CategoryChange, true, ActionConnectionSecretSet)
	c.registerAll(CategoryAdmin, false, ActionRoleBindingSet, ActionRoleBindingRemoved, ActionUserUpdated,
		ActionTeamCreated, ActionTeamUpdated, ActionTeamDeleted, ActionGrantAdded, ActionGrantRemoved,
		ActionSettingsUpdated, ActionUserBootstrapAdmin, ActionDevSeed, ActionUserInvited, ActionInvitationAccepted)
	c.registerAll(CategoryAccess, false, ActionAuthLogin, ActionAuthLoginFailed, ActionAuthLogout, ActionSampleViewed,
		ActionDLQViewed)
	c.registerAll(CategoryAccess, true, ActionSampleViewedRaw, ActionLiveTailOpened, ActionDLQExported,
		ActionSecretRead, ActionAuditExported)
	c.registerPasswordActions()
	return c
}

// Register adds or overrides one action.
func (c *Catalog) Register(a Action, meta ActionMeta) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.actions[a] = meta
}

// Describe returns the metadata of an action.
func (c *Catalog) Describe(a Action) (ActionMeta, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	meta, ok := c.actions[a]
	return meta, ok
}

// Actions lists every catalogued action in lexical order.
func (c *Catalog) Actions() []Action {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Action, 0, len(c.actions))
	for a := range c.actions {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (c *Catalog) registerAll(cat Category, sensitive bool, actions ...Action) {
	for _, a := range actions {
		c.Register(a, ActionMeta{Sensitive: sensitive, Category: cat})
	}
}

// CatalogHook is the recorder EntryHook that rejects unknown actions and stamps sensitivity.
type CatalogHook struct {
	catalog *Catalog
}

// NewCatalogHook builds the hook over a catalog.
func NewCatalogHook(c *Catalog) *CatalogHook {
	return &CatalogHook{catalog: c}
}

// Before implements EntryHook.
func (h *CatalogHook) Before(_ context.Context, e Entry) (Entry, error) {
	meta, ok := h.catalog.Describe(e.Action)
	if !ok {
		return e, ErrUnknownAction
	}
	e.Sensitive = e.Sensitive || meta.Sensitive
	return e, nil
}
