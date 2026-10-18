package connections

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/pkg/kafka"
)

// Audit actions of the connections feature (names follow the access-control catalog).
const (
	ActionCreated        audit.Action = "connection.created"
	ActionUpdated        audit.Action = "connection.updated"
	ActionDeleted        audit.Action = "connection.deleted"
	ActionSecretReplaced audit.Action = "connection.secret_replaced"
	ActionTested         audit.Action = "connection.tested"
	ActionTopicCreated   audit.Action = "topic.created"
)

// AuditView is the audited shape of a connection: never secret values, only which fields are set.
type AuditView struct {
	Name             string                       `json:"name"`
	Description      string                       `json:"description"`
	EnvTag           EnvTag                       `json:"envTag"`
	TeamID           uuid.UUID                    `json:"teamId"`
	BootstrapServers []string                     `json:"bootstrapServers"`
	SecurityMode     kafka.SecurityMode           `json:"securityMode"`
	TLSEnabled       bool                         `json:"tlsEnabled"`
	Username         string                       `json:"username,omitempty"`
	ClientID         string                       `json:"clientId,omitempty"`
	TokenURL         string                       `json:"tokenUrl,omitempty"`
	SchemaRegistry   *kafka.SchemaRegistrySpec    `json:"schemaRegistry,omitempty"`
	Properties       map[kafka.PropertyKey]string `json:"properties,omitempty"`
	SecretFields     []kafka.SecretField          `json:"secretFields,omitempty"`
	Sensitive        bool                         `json:"sensitive"`
	AllowTopicCreate bool                         `json:"allowTopicCreate"`
	ProbeEnabled     bool                         `json:"probeEnabled"`
}

// ViewOf returns the audited view of c.
func ViewOf(c Connection) *AuditView {
	return &AuditView{
		Name: c.Name, Description: c.Description, EnvTag: c.EnvTag, TeamID: c.TeamID, BootstrapServers: c.BootstrapServers,
		SecurityMode: c.Security.Mode, TLSEnabled: c.Security.TLS.Enabled, Username: c.Security.Auth.Username,
		ClientID: c.Security.Auth.ClientID, TokenURL: c.Security.Auth.TokenURL, SchemaRegistry: c.SchemaRegistry,
		Properties: c.Properties, SecretFields: c.SecretFields, Sensitive: c.Sensitive,
		AllowTopicCreate: c.AllowTopicCreate, ProbeEnabled: c.ProbeEnabled,
	}
}

// TestAudit is the audited outcome of a test run.
type TestAudit struct {
	OK     bool         `json:"ok"`
	Health HealthStatus `json:"health"`
	Draft  bool         `json:"draft"`
}

// SecretAudit names the replaced secret fields (never their values).
type SecretAudit struct {
	Replaced []kafka.SecretField `json:"replacedFields"`
}

// TopicAudit is the audited create-topic request.
type TopicAudit struct {
	Topic       string `json:"topic"`
	Partitions  int32  `json:"partitions"`
	RetentionMs int64  `json:"retentionMs"`
}

// Auditor records connection changes through the request's audit Scope (same transaction).
type Auditor struct{}

// Record writes one change; requests without a scope (e.g. the prober) are not audited here.
func (Auditor) Record(ctx context.Context, action audit.Action, c Connection, before, after any) error {
	scope, ok := audit.FromContext(ctx)
	if !ok {
		return nil
	}
	change := audit.Change{Action: action, Object: audit.ObjectConnection, ObjectID: string(DraftObjectID), Before: before, After: after}
	if c.ID != uuid.Nil {
		change.ObjectID = c.ID.String()
	}
	if c.TeamID != uuid.Nil {
		team := c.TeamID
		change.TeamID = &team
	}
	return scope.Record(ctx, change)
}

// AuditObject names placeholder object ids.
type AuditObject string

// DraftObjectID is the audit object id of tests run on an unsaved draft.
const DraftObjectID AuditObject = "draft"
