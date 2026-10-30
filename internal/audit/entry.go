// Package audit is the append-only audit trail. platform-core owns the write side (entry, recorder,
// scope, diff, middleware); access-control adds the action catalog and the read/export side.
package audit

import (
	"encoding/json"

	"github.com/google/uuid"
)

// Action names an audited operation as "<object>.<verb>"; access-control's Catalog lists all of them.
type Action string

const (
	ActionAuthLogin          Action = "auth.login"
	ActionAuthLoginFailed    Action = "auth.login_failed"
	ActionAuthLogout         Action = "auth.logout"
	ActionUserBootstrapAdmin Action = "user.bootstrap_admin"
	ActionDevSeed            Action = "dev.seed"
	ActionSecretRead         Action = "secret.read_by_service"
	ActionSecretsRotated     Action = "secrets.rotated"
)

// ObjectType names the kind of object an audit entry refers to.
type ObjectType string

const (
	ObjectConnection   ObjectType = "connection"
	ObjectRoute        ObjectType = "route"
	ObjectRouteVersion ObjectType = "route_version"
	ObjectDeployment   ObjectType = "deployment"
	ObjectTransform    ObjectType = "transform"
	ObjectLookupTable  ObjectType = "lookup_table"
	ObjectLookupSource ObjectType = "lookup_source"
	ObjectUser         ObjectType = "user"
	ObjectTeam         ObjectType = "team"
	ObjectRoleBinding  ObjectType = "role_binding"
	ObjectAlertRule    ObjectType = "alert_rule"
	ObjectSecret       ObjectType = "secret"
	ObjectSession      ObjectType = "session"
	ObjectAPIToken     ObjectType = "api_token"
	ObjectRequest      ObjectType = "request"
)

// ActorKind tells who performed an action.
type ActorKind string

const (
	ActorUser     ActorKind = "user"
	ActorAPIToken ActorKind = "api_token"
	ActorWorkload ActorKind = "workload"
	ActorSystem   ActorKind = "system"
)

// Actor identifies the principal behind an entry without depending on the auth package.
type Actor struct {
	UserID *uuid.UUID
	Kind   ActorKind
	Name   string
}

// Entry is one audit_log row.
type Entry struct {
	Actor       Actor
	Action      Action
	Object      ObjectType
	ObjectID    string
	TeamID      *uuid.UUID
	Diff        json.RawMessage
	IP          string
	RequestID   string
	Sensitive   bool
	ObjectLabel string
}

// RequestContext carries the per-request audit context (who, from where, which request).
type RequestContext struct {
	Actor     Actor
	IP        string
	RequestID string
}

// Apply copies the request context into an entry where the entry does not set it.
func (rc RequestContext) Apply(e Entry) Entry {
	if e.Actor.Kind == "" {
		e.Actor = rc.Actor
	}
	if e.IP == "" {
		e.IP = rc.IP
	}
	if e.RequestID == "" {
		e.RequestID = rc.RequestID
	}
	return e
}
