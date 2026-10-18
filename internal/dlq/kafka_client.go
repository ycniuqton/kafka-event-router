package dlq

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/pkg/kafka"
)

// ClientOpener opens raw franz-go clients for a saved connection with extra options (assignments).
type ClientOpener interface {
	Open(ctx context.Context, connectionID string, purpose kafka.ClientPurpose, extra ...kgo.Opt) (*kgo.Client, error)
}

// SpecLookup resolves a connection id to its client spec with decrypted secrets.
type SpecLookup interface {
	Spec(ctx context.Context, connectionID string) (kafka.ConnectionSpec, error)
}

// OptionsBuilder turns a spec into franz-go options (implemented by kafka.FranzClientFactory).
type OptionsBuilder interface {
	Options(spec kafka.ConnectionSpec, purpose kafka.ClientPurpose) ([]kgo.Opt, error)
}

// KindOptions builds options for one non-default cluster kind (the in-process demo cluster starts lazily).
type KindOptions interface {
	Options(ctx context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose) ([]kgo.Opt, error)
}

// SpecClientOpener opens clients from resolved specs through the pkg/kafka option builder; Kinds selects
// the builder of other cluster kinds (kafka.ClusterDemo) by the spec's kind.
type SpecClientOpener struct {
	Specs   SpecLookup
	Options OptionsBuilder
	Kinds   map[kafka.ClusterKind]KindOptions
}

// Open implements ClientOpener; resolution or option failures are upstream errors.
func (o SpecClientOpener) Open(ctx context.Context, connectionID string, purpose kafka.ClientPurpose, extra ...kgo.Opt) (*kgo.Client, error) {
	spec, err := o.Specs.Spec(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	opts, err := o.options(ctx, spec, purpose)
	if err != nil {
		return nil, ErrUpstream.Wrap(err)
	}
	cl, err := kgo.NewClient(append(opts, extra...)...)
	if err != nil {
		return nil, ErrUpstream.Wrap(err)
	}
	return cl, nil
}

// options returns the options of spec's cluster kind (the default builder for real Kafka).
func (o SpecClientOpener) options(ctx context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose) ([]kgo.Opt, error) {
	if k, ok := o.Kinds[spec.ClusterKindOrDefault()]; ok {
		return k.Options(ctx, spec, purpose)
	}
	return o.Options.Options(spec, purpose)
}

// SeedClientOpener opens plaintext clients against fixed seeds for every connection (tests, local dev).
type SeedClientOpener struct {
	Seeds []string
}

// Open implements ClientOpener.
func (o SeedClientOpener) Open(_ context.Context, _ string, _ kafka.ClientPurpose, extra ...kgo.Opt) (*kgo.Client, error) {
	cl, err := kgo.NewClient(append([]kgo.Opt{kgo.SeedBrokers(o.Seeds...)}, extra...)...)
	if err != nil {
		return nil, fmt.Errorf("dlq: client: %w", err)
	}
	return cl, nil
}

// ConnectionSpecs adapts the connections feature's SpecResolver to SpecLookup.
type ConnectionSpecs struct {
	Resolver *connections.SpecResolver
}

// Spec implements SpecLookup; an unknown id is a missing route target (404).
func (c ConnectionSpecs) Spec(ctx context.Context, connectionID string) (kafka.ConnectionSpec, error) {
	id, err := uuid.Parse(connectionID)
	if err != nil {
		return kafka.ConnectionSpec{}, ErrUpstream.Wrap(err)
	}
	_, spec, err := c.Resolver.Resolve(ctx, id)
	if err != nil {
		return kafka.ConnectionSpec{}, ErrUpstream.Wrap(err)
	}
	return spec, nil
}

// SandboxKindOptions lets clients of an in-process sandbox cluster (kafka.ClusterDemo) create topics on
// first use, so demo retries reach destinations the API process has not created yet.
type SandboxKindOptions struct {
	Inner KindOptions
}

// Options implements KindOptions.
func (s SandboxKindOptions) Options(ctx context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose) ([]kgo.Opt, error) {
	opts, err := s.Inner.Options(ctx, spec, purpose)
	if err != nil {
		return nil, err
	}
	return append(opts, kgo.AllowAutoTopicCreation()), nil
}
