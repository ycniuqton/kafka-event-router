// Package connhttp exposes the connections API: class-based handlers for CRUD, live tests (NDJSON),
// the topic browser, topic creation and the workload-only spec endpoint.
package connhttp

import (
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/pkg/kafka"
)

// SecretState is the only thing the API ever says about a secret.
type SecretState string

const (
	SecretSet     SecretState = "set"
	SecretMissing SecretState = "missing"
)

// SecretFieldsOfMode lists which secret fields each security mode uses (shown as set/missing).
var SecretFieldsOfMode = connections.SecretFieldsOfMode

// StateOf renders whether a secret field is stored.
func StateOf(stored bool) SecretState {
	if stored {
		return SecretSet
	}
	return SecretMissing
}

// OwnerDTO is the owner team of a connection.
type OwnerDTO struct {
	TeamID   uuid.UUID `json:"teamId"`
	TeamName string    `json:"teamName,omitempty"`
}

// SecurityDTO is the public security block (never secret values).
type SecurityDTO struct {
	Mode         kafka.SecurityMode                `json:"mode"`
	TLS          kafka.TLSSpec                     `json:"tls"`
	Username     string                            `json:"username,omitempty"`
	ClientID     string                            `json:"clientId,omitempty"`
	TokenURL     string                            `json:"tokenUrl,omitempty"`
	Scopes       []string                          `json:"scopes,omitempty"`
	SecretFields map[kafka.SecretField]SecretState `json:"secretFields"`
}

// SchemaRegistryDTO is the public registry block; the password or token is only "set" / "missing".
type SchemaRegistryDTO struct {
	URL    string                 `json:"url"`
	Auth   kafka.RegistryAuthSpec `json:"auth"`
	Secret *SecretState           `json:"secret,omitempty"`
}

// UsedByDTO counts (and on GET-one lists) the routes using a connection.
type UsedByDTO struct {
	Count  int                    `json:"count"`
	Routes []connections.RouteRef `json:"routes,omitempty"`
}

// ConnectionDTO is the public connection resource.
type ConnectionDTO struct {
	ID               uuid.UUID                    `json:"id"`
	Name             string                       `json:"name"`
	Description      string                       `json:"description"`
	EnvTag           connections.EnvTag           `json:"envTag"`
	Owner            OwnerDTO                     `json:"owner"`
	BootstrapServers []string                     `json:"bootstrapServers"`
	Security         SecurityDTO                  `json:"security"`
	SchemaRegistry   *SchemaRegistryDTO           `json:"schemaRegistry"`
	Properties       map[kafka.PropertyKey]string `json:"properties"`
	Sensitive        bool                         `json:"sensitive"`
	AllowTopicCreate bool                         `json:"allowTopicCreate"`
	ProbeEnabled     bool                         `json:"probeEnabled"`
	Kind             kafka.ClusterKind            `json:"kind"`
	Health           connections.Health           `json:"health"`
	UsedBy           UsedByDTO                    `json:"usedBy"`
	Version          int64                        `json:"version"`
	CreatedAt        time.Time                    `json:"createdAt"`
	UpdatedAt        time.Time                    `json:"updatedAt"`
}

// Presenter maps entities to DTOs.
type Presenter struct{}

// Connection renders c with its usage.
func (Presenter) Connection(c connections.Connection, used UsedByDTO) ConnectionDTO {
	props := c.Properties
	if props == nil {
		props = map[kafka.PropertyKey]string{}
	}
	return ConnectionDTO{
		ID: c.ID, Name: c.Name, Description: c.Description, EnvTag: c.EnvTag, Owner: OwnerDTO{TeamID: c.TeamID, TeamName: c.TeamName},
		BootstrapServers: c.BootstrapServers, Security: Presenter{}.security(c), SchemaRegistry: Presenter{}.registry(c), Properties: props,
		Sensitive: c.Sensitive, AllowTopicCreate: c.AllowTopicCreate, ProbeEnabled: c.ProbeEnabled, Kind: c.Kind,
		Health: c.Health, UsedBy: used, Version: int64(c.Version), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

func (Presenter) security(c connections.Connection) SecurityDTO {
	fields := map[kafka.SecretField]SecretState{}
	for _, f := range SecretFieldsOfMode[c.Security.Mode] {
		fields[f] = StateOf(c.HasSecret(f))
	}
	a := c.Security.Auth
	return SecurityDTO{Mode: c.Security.Mode, TLS: c.Security.TLS, Username: a.Username, ClientID: a.ClientID, TokenURL: a.TokenURL, Scopes: a.Scopes, SecretFields: fields}
}

func (Presenter) registry(c connections.Connection) *SchemaRegistryDTO {
	if c.SchemaRegistry == nil {
		return nil
	}
	dto := &SchemaRegistryDTO{URL: c.SchemaRegistry.URL, Auth: c.SchemaRegistry.Auth}
	dto.Auth.Mode = c.SchemaRegistry.Auth.ModeOrDefault()
	if (connections.SecretUsage{}).RegistryNeedsSecret(c.Spec(kafka.SecretBundle{})) {
		state := StateOf(c.HasSecret(kafka.SecretRegistry))
		dto.Secret = &state
	}
	return dto
}

// SecurityInput is the writable security block.
type SecurityInput struct {
	Mode     kafka.SecurityMode `json:"mode"`
	TLS      kafka.TLSSpec      `json:"tls"`
	Username string             `json:"username,omitempty"`
	ClientID string             `json:"clientId,omitempty"`
	TokenURL string             `json:"tokenUrl,omitempty"`
	Scopes   []string           `json:"scopes,omitempty"`
}

// Auth returns the domain identity of the input.
func (in SecurityInput) Auth() kafka.AuthSpec {
	return kafka.AuthSpec{Username: in.Username, ClientID: in.ClientID, TokenURL: in.TokenURL, Scopes: in.Scopes}
}

// SchemaRegistryInput is the writable registry block (null removes the registry).
type SchemaRegistryInput struct {
	URL  string                 `json:"url"`
	Auth kafka.RegistryAuthSpec `json:"auth"`
}

// SecretsInput carries write-only secrets; absent keys keep stored values.
type SecretsInput struct {
	Password             string `json:"password,omitempty"`
	ClientKeyPEM         string `json:"clientKeyPem,omitempty"`
	ClientSecret         string `json:"clientSecret,omitempty"`
	SchemaRegistrySecret string `json:"schemaRegistrySecret,omitempty"`
}

// OwnerInput selects the owner team.
type OwnerInput struct {
	TeamID uuid.UUID `json:"teamId"`
}

// ConnectionInput is the create body and the merge-patch target.
type ConnectionInput struct {
	Name             string                       `json:"name"`
	Description      string                       `json:"description"`
	EnvTag           connections.EnvTag           `json:"envTag"`
	Owner            *OwnerInput                  `json:"owner,omitempty"`
	BootstrapServers []string                     `json:"bootstrapServers"`
	Security         SecurityInput                `json:"security"`
	SchemaRegistry   *SchemaRegistryInput         `json:"schemaRegistry,omitempty"`
	Properties       map[kafka.PropertyKey]string `json:"properties,omitempty"`
	Sensitive        bool                         `json:"sensitive"`
	AllowTopicCreate bool                         `json:"allowTopicCreate"`
	ProbeEnabled     *bool                        `json:"probeEnabled,omitempty"`
	Kind             kafka.ClusterKind            `json:"kind,omitempty"`
	Secrets          *SecretsInput                `json:"secrets,omitempty"`
}

// Draft converts the input into a domain draft for the given team.
func (in ConnectionInput) Draft(team uuid.UUID) connections.Draft {
	d := connections.Draft{
		TeamID: team, Name: in.Name, Description: in.Description, EnvTag: in.EnvTag, BootstrapServers: in.BootstrapServers,
		Security:   connections.Security{Mode: in.Security.Mode, TLS: in.Security.TLS, Auth: in.Security.Auth()},
		Properties: in.Properties, Sensitive: in.Sensitive, AllowTopicCreate: in.AllowTopicCreate, ProbeEnabled: true, Kind: in.Kind,
	}
	if in.ProbeEnabled != nil {
		d.ProbeEnabled = *in.ProbeEnabled
	}
	if in.Secrets != nil {
		d.Secrets = kafka.SecretBundle{Password: in.Secrets.Password, ClientKeyPEM: in.Secrets.ClientKeyPEM, ClientSecret: in.Secrets.ClientSecret,
			RegistrySecret: in.Secrets.SchemaRegistrySecret}
	}
	if in.SchemaRegistry != nil {
		d.SchemaRegistry = &kafka.SchemaRegistrySpec{URL: in.SchemaRegistry.URL, Auth: in.SchemaRegistry.Auth}
	}
	return d
}

// InputOf renders a stored connection as the patch target (no secrets; the kind is immutable and kept by the service).
func InputOf(c connections.Connection) ConnectionInput {
	probe, a := c.ProbeEnabled, c.Security.Auth
	in := ConnectionInput{
		Name: c.Name, Description: c.Description, EnvTag: c.EnvTag, BootstrapServers: c.BootstrapServers,
		Security:   SecurityInput{Mode: c.Security.Mode, TLS: c.Security.TLS, Username: a.Username, ClientID: a.ClientID, TokenURL: a.TokenURL, Scopes: a.Scopes},
		Properties: c.Properties, Sensitive: c.Sensitive, AllowTopicCreate: c.AllowTopicCreate, ProbeEnabled: &probe,
	}
	if c.SchemaRegistry != nil {
		in.SchemaRegistry = &SchemaRegistryInput{URL: c.SchemaRegistry.URL, Auth: c.SchemaRegistry.Auth}
	}
	return in
}

// TestOptionsInput is the body of POST /connections/{id}/test.
type TestOptionsInput struct {
	ReadTopic  string `json:"readTopic,omitempty"`
	WriteTopic string `json:"writeTopic,omitempty"`
}

// DraftTestInput is the body of POST /connections/test.
type DraftTestInput struct {
	ConnectionInput
	TestOptionsInput
	ConnectionID *uuid.UUID `json:"connectionId,omitempty"`
}

// TopicInput is the body of POST /connections/{id}/topics.
type TopicInput struct {
	Name              string            `json:"name"`
	Partitions        *int32            `json:"partitions,omitempty"`
	ReplicationFactor *int16            `json:"replicationFactor,omitempty"`
	RetentionMs       *int64            `json:"retentionMs,omitempty"`
	Configs           map[string]string `json:"configs,omitempty"`
}
