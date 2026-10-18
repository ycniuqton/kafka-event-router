package checks

import (
	"context"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/pkg/schemaregistry"
)

// SchemaRegistryCheck verifies that the connection's schema registry answers and accepts the credentials.
type SchemaRegistryCheck struct {
	Client  *schemaregistry.Client
	Reasons schemaregistry.StatusReasons
}

// NewSchemaRegistryCheck builds the check with the default auth modes and status table.
func NewSchemaRegistryCheck() SchemaRegistryCheck {
	return SchemaRegistryCheck{Client: schemaregistry.NewClient(nil, schemaregistry.DefaultAuthRegistry()), Reasons: schemaregistry.DefaultStatusReasons}
}

// Step implements connections.TestCheck.
func (SchemaRegistryCheck) Step() connections.TestStep { return connections.StepSchemaRegistry }

// Run implements connections.TestCheck; connections without a registry skip it.
func (c SchemaRegistryCheck) Run(ctx context.Context, s *connections.TestSession) connections.StepOutcome {
	if s.Spec.SchemaRegistry == nil {
		return connections.Skipped()
	}
	err := c.Client.Ping(ctx, s.Spec)
	if err == nil {
		return connections.Passed()
	}
	if reason, ok := c.Reasons.ReasonOf(err); ok {
		return connections.FailedAs(reason, err)
	}
	return connections.Failed(err)
}
