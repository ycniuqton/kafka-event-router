package lookups

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/pkg/lookup"
)

// Audit actions and object type of lookup sources (registered in the access-control catalog at wiring).
const (
	ActionCreated        audit.Action = "lookup_source.created"
	ActionUpdated        audit.Action = "lookup_source.updated"
	ActionDeleted        audit.Action = "lookup_source.deleted"
	ActionSecretReplaced audit.Action = "lookup_source.secret_replaced"
	ActionTested         audit.Action = "lookup_source.tested"

	ObjectLookupSource = audit.ObjectLookupSource
)

// Actions lists every audit action of the feature.
var Actions = []audit.Action{ActionCreated, ActionUpdated, ActionDeleted, ActionSecretReplaced, ActionTested}

// AuditObject names placeholder object ids.
type AuditObject string

// DraftObjectID is the audit object id of tests run on an unsaved draft.
const DraftObjectID AuditObject = "draft"

// AuditView is the audited shape of a source: kind settings and which secrets are set, never their values.
type AuditView struct {
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	Kind         lookup.SourceKind    `json:"kind"`
	EnvTag       EnvTag               `json:"envTag"`
	TeamID       uuid.UUID            `json:"teamId"`
	Sensitive    bool                 `json:"sensitive"`
	Settings     lookup.Settings      `json:"settings"`
	Limits       lookup.Limits        `json:"limits"`
	SecretFields []lookup.SecretField `json:"secretFields,omitempty"`
}

// ViewOf returns the audited view of s.
func ViewOf(s Source) *AuditView {
	return &AuditView{Name: s.Name, Description: s.Description, Kind: s.Kind, EnvTag: s.EnvTag, TeamID: s.TeamID,
		Sensitive: s.Sensitive, Settings: s.Settings, Limits: s.Limits, SecretFields: s.SecretFields}
}

// TestAudit is the audited outcome of a test run.
type TestAudit struct {
	OK     bool         `json:"ok"`
	Health HealthStatus `json:"health"`
	Draft  bool         `json:"draft"`
}

// SecretAudit names the replaced secret fields.
type SecretAudit struct {
	Replaced []lookup.SecretField `json:"replacedFields"`
}

// Auditor records changes through the request's audit Scope (same transaction).
type Auditor struct{}

// Record writes one change; calls without a scope are not audited here.
func (Auditor) Record(ctx context.Context, action audit.Action, s Source, before, after any) error {
	scope, ok := audit.FromContext(ctx)
	if !ok {
		return nil
	}
	change := audit.Change{Action: action, Object: ObjectLookupSource, ObjectID: string(DraftObjectID), Before: before, After: after}
	if s.ID != uuid.Nil {
		change.ObjectID = s.ID.String()
	}
	if s.TeamID != uuid.Nil {
		team := s.TeamID
		change.TeamID = &team
	}
	return scope.Record(ctx, change)
}
