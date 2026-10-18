// Package checks holds the connection test checklist steps; each check is one class registered by step.
package checks

import (
	"context"
	"errors"
	"io"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/pkg/kafka"
)

// DefaultRegistry registers every check (the schema registry check runs only for connections with a registry).
func DefaultRegistry() *connections.CheckRegistry {
	return connections.NewCheckRegistry(
		ReachabilityCheck{Dialer: kafka.SeedDialer{}},
		AuthenticationCheck{},
		ListTopicsCheck{},
		ReadCheck{Reader: SamplerRecordReader{}},
		WriteCheck{},
		NewSchemaRegistryCheck(),
	)
}

// ReachabilityCheck verifies that at least one seed the client dials accepts TCP connections.
type ReachabilityCheck struct {
	Dialer kafka.SeedDialer
}

// Step implements connections.TestCheck.
func (ReachabilityCheck) Step() connections.TestStep { return connections.StepBrokersReachable }

// Run implements connections.TestCheck.
func (c ReachabilityCheck) Run(ctx context.Context, s *connections.TestSession) connections.StepOutcome {
	if err := c.Dialer.Reach(ctx, s.Client.Seeds()); err != nil {
		return connections.Failed(err)
	}
	return connections.Passed()
}

// AuthenticationCheck sends ApiVersions through the configured TLS/SASL handshake.
type AuthenticationCheck struct{}

// Step implements connections.TestCheck.
func (AuthenticationCheck) Step() connections.TestStep { return connections.StepAuthenticated }

// Run implements connections.TestCheck; a connection closed during the handshake counts as auth failure.
func (AuthenticationCheck) Run(ctx context.Context, s *connections.TestSession) connections.StepOutcome {
	err := s.Client.Admin().Ping(ctx)
	if err == nil {
		return connections.Passed()
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return connections.FailedAs(kafka.ReasonAuthFailed, err)
	}
	return connections.Failed(err)
}

// ListTopicsCheck verifies that the user may describe the cluster's topics.
type ListTopicsCheck struct{}

// Step implements connections.TestCheck.
func (ListTopicsCheck) Step() connections.TestStep { return connections.StepListTopics }

// Run implements connections.TestCheck.
func (ListTopicsCheck) Run(ctx context.Context, s *connections.TestSession) connections.StepOutcome {
	if _, err := s.Client.Admin().ListTopics(ctx); err != nil {
		return connections.Failed(err)
	}
	return connections.Passed()
}
