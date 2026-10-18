package codec

import (
	"context"
	"errors"
	"fmt"
)

// ErrTransient marks failures that may succeed on retry (registry outage); the pipeline
// maps them to decode_transient so the runtime backs off instead of dead-lettering.
var ErrTransient = errors.New("transient failure")

// ErrSchemaNotFound reports an unknown schema id, subject or version (permanent).
var ErrSchemaNotFound = errors.New("schema not found")

// ErrRegistryNotConfigured reports a registry-backed format on a connection without a registry.
var ErrRegistryNotConfigured = errors.New("no schema registry configured for this connection")

// SchemaType is the registry schema language.
type SchemaType string

const (
	SchemaTypeAvro     SchemaType = "AVRO"
	SchemaTypeProtobuf SchemaType = "PROTOBUF"
	SchemaTypeJSON     SchemaType = "JSON"
)

// SchemaReference is an import of another registered schema (protobuf imports, avro named types).
type SchemaReference struct {
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Version int    `json:"version"`
}

// RegisteredSchema is one schema version as the registry returns it.
type RegisteredSchema struct {
	ID         int32             `json:"id"`
	Subject    string            `json:"subject,omitempty"`
	Version    int               `json:"version,omitempty"`
	Type       SchemaType        `json:"schemaType,omitempty"`
	Text       string            `json:"schema"`
	References []SchemaReference `json:"references,omitempty"`
}

// EffectiveType returns the schema type; the registry omits it for Avro.
func (s RegisteredSchema) EffectiveType() SchemaType {
	if s.Type == "" {
		return SchemaTypeAvro
	}
	return s.Type
}

// SchemaRegistry is the port to a Confluent-compatible schema registry of one connection.
type SchemaRegistry interface {
	SchemaByID(ctx context.Context, id int32) (RegisteredSchema, error)
	Latest(ctx context.Context, subject string) (RegisteredSchema, error)
	Version(ctx context.Context, subject string, version int) (RegisteredSchema, error)
}

// RegistryProvider returns the registry of a connection; it is injected at bootstrap
// (connections owns the per-connection registry settings).
type RegistryProvider interface {
	Registry(ctx context.Context, connectionID string) (SchemaRegistry, error)
}

// UnconfiguredRegistry fails every call with ErrRegistryNotConfigured.
type UnconfiguredRegistry struct{}

func (UnconfiguredRegistry) SchemaByID(context.Context, int32) (RegisteredSchema, error) {
	return RegisteredSchema{}, ErrRegistryNotConfigured
}

func (UnconfiguredRegistry) Latest(context.Context, string) (RegisteredSchema, error) {
	return RegisteredSchema{}, ErrRegistryNotConfigured
}

func (UnconfiguredRegistry) Version(context.Context, string, int) (RegisteredSchema, error) {
	return RegisteredSchema{}, ErrRegistryNotConfigured
}

// ConnectionRegistry resolves the registry of one connection on every call, so a compile
// never touches the network and registry settings changes apply without recompiling.
type ConnectionRegistry struct {
	Provider     RegistryProvider
	ConnectionID string
}

func (r ConnectionRegistry) resolve(ctx context.Context) (SchemaRegistry, error) {
	if r.Provider == nil {
		return UnconfiguredRegistry{}, nil
	}
	reg, err := r.Provider.Registry(ctx, r.ConnectionID)
	if err != nil {
		return nil, fmt.Errorf("%w: schema registry of connection %s: %v", ErrTransient, r.ConnectionID, err)
	}
	if reg == nil {
		return UnconfiguredRegistry{}, nil
	}
	return reg, nil
}

func (r ConnectionRegistry) SchemaByID(ctx context.Context, id int32) (RegisteredSchema, error) {
	reg, err := r.resolve(ctx)
	if err != nil {
		return RegisteredSchema{}, err
	}
	return reg.SchemaByID(ctx, id)
}

func (r ConnectionRegistry) Latest(ctx context.Context, subject string) (RegisteredSchema, error) {
	reg, err := r.resolve(ctx)
	if err != nil {
		return RegisteredSchema{}, err
	}
	return reg.Latest(ctx, subject)
}

func (r ConnectionRegistry) Version(ctx context.Context, subject string, version int) (RegisteredSchema, error) {
	reg, err := r.resolve(ctx)
	if err != nil {
		return RegisteredSchema{}, err
	}
	return reg.Version(ctx, subject, version)
}
