package connections

import (
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/kafka"
)

// LastFailure explains the first failed step of the last check.
type LastFailure struct {
	Step    TestStep            `json:"step"`
	Reason  kafka.FailureReason `json:"reason"`
	Message string              `json:"message"`
	Hint    string              `json:"hint"`
}

// Health is the result of the last test or probe.
type Health struct {
	Status      HealthStatus `json:"status"`
	CheckedAt   *time.Time   `json:"checkedAt"`
	LastFailure *LastFailure `json:"lastFailure"`
}

// Security is the non-secret security configuration of a connection.
type Security struct {
	Mode kafka.SecurityMode
	TLS  kafka.TLSSpec
	Auth kafka.AuthSpec
}

// StoredConfig is the `connections.config` JSONB document (never contains secret values).
type StoredConfig struct {
	TLS          kafka.TLSSpec                `json:"tls"`
	Auth         kafka.AuthSpec               `json:"auth"`
	Properties   map[kafka.PropertyKey]string `json:"properties,omitempty"`
	SecretFields []kafka.SecretField          `json:"secretFields,omitempty"`
}

// Connection is a saved Kafka cluster.
type Connection struct {
	ID               uuid.UUID
	TeamID           uuid.UUID
	TeamName         string
	Name             string
	Description      string
	EnvTag           EnvTag
	BootstrapServers []string
	Security         Security
	Properties       map[kafka.PropertyKey]string
	SchemaRegistry   *kafka.SchemaRegistrySpec
	SecretRef        secrets.Ref
	SecretFields     []kafka.SecretField
	Sensitive        bool
	AllowTopicCreate bool
	ProbeEnabled     bool
	Kind             kafka.ClusterKind
	Health           Health
	Version          store.RowVersion
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Spec builds the client spec of the connection with the given decrypted secrets.
func (c Connection) Spec(bundle kafka.SecretBundle) kafka.ConnectionSpec {
	return kafka.ConnectionSpec{
		ID: c.ID.String(), Kind: c.Kind, BootstrapServers: c.BootstrapServers, SecurityMode: c.Security.Mode,
		TLS: c.Security.TLS, Auth: c.Security.Auth, Properties: c.Properties, SchemaRegistry: c.SchemaRegistry, Secret: bundle,
	}
}

// Config returns the JSONB config document of the connection.
func (c Connection) Config() StoredConfig {
	return StoredConfig{TLS: c.Security.TLS, Auth: c.Security.Auth, Properties: c.Properties, SecretFields: c.SecretFields}
}

// HasSecret reports whether a secret field is stored.
func (c Connection) HasSecret(f kafka.SecretField) bool {
	for _, s := range c.SecretFields {
		if s == f {
			return true
		}
	}
	return false
}

// TransportChanged reports whether brokers, security or the schema registry differ (health must be re-checked).
func (c Connection) TransportChanged(other Connection) bool {
	if c.Security.Mode != other.Security.Mode || c.Security.TLS != other.Security.TLS || !c.Security.Auth.Equal(other.Security.Auth) {
		return true
	}
	if RegistryOf(c.SchemaRegistry) != RegistryOf(other.SchemaRegistry) {
		return true
	}
	if len(c.BootstrapServers) != len(other.BootstrapServers) {
		return true
	}
	for i := range c.BootstrapServers {
		if c.BootstrapServers[i] != other.BootstrapServers[i] {
			return true
		}
	}
	return false
}

// Draft is the desired state of a connection (create, merged update, or unsaved test).
type Draft struct {
	TeamID           uuid.UUID
	Name             string
	Description      string
	EnvTag           EnvTag
	BootstrapServers []string
	Security         Security
	Properties       map[kafka.PropertyKey]string
	SchemaRegistry   *kafka.SchemaRegistrySpec
	Secrets          kafka.SecretBundle
	Sensitive        bool
	AllowTopicCreate bool
	ProbeEnabled     bool
	Kind             kafka.ClusterKind
}

// Spec builds the client spec of the draft (secrets as given).
func (d Draft) Spec() kafka.ConnectionSpec {
	return kafka.ConnectionSpec{
		Kind: d.KindOrDefault(), BootstrapServers: d.BootstrapServers, SecurityMode: d.Security.Mode,
		TLS: d.Security.TLS, Auth: d.Security.Auth, Properties: d.Properties, SchemaRegistry: d.SchemaRegistry, Secret: d.Secrets,
	}
}

// KindOrDefault defaults the cluster kind to kafka.
func (d Draft) KindOrDefault() kafka.ClusterKind {
	if d.Kind == "" {
		return kafka.ClusterKafka
	}
	return d.Kind
}

// SecretFieldsOf lists the fields set in a bundle, in stable order.
type SecretFieldsOf kafka.SecretBundle

// List returns the set fields.
func (b SecretFieldsOf) List() []kafka.SecretField {
	bundle := kafka.SecretBundle(b)
	var out []kafka.SecretField
	for _, f := range kafka.SecretFields {
		if bundle.Has(f) {
			out = append(out, f)
		}
	}
	return out
}

// RegistryOf is a comparable view of an optional schema registry (zero when absent).
func RegistryOf(r *kafka.SchemaRegistrySpec) kafka.SchemaRegistrySpec {
	if r == nil {
		return kafka.SchemaRegistrySpec{}
	}
	return *r
}

// ListQuery filters and pages the connection list.
type ListQuery struct {
	Env    EnvTag
	Status HealthStatus
	Text   string
	Limit  int
	After  *store.Cursor
}

// Page is one page of connections.
type Page struct {
	Items []Connection
	Next  *store.Cursor
}

// RouteRef names a route using a connection.
type RouteRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}
