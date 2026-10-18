package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Client is an open cluster client: the raw franz-go client plus admin operations.
type Client interface {
	Raw() *kgo.Client
	Admin() Admin
	Seeds() []string
	Close()
}

// ClientFactory is the base of every cluster-kind factory.
type ClientFactory interface {
	Kind() ClusterKind
	NewClient(ctx context.Context, spec ConnectionSpec, purpose ClientPurpose) (Client, error)
}

// PurposeTiming holds purpose default timings.
type PurposeTiming time.Duration

const (
	SamplerFetchMaxWait PurposeTiming = PurposeTiming(500 * time.Millisecond)
	AdminRetryTimeout   PurposeTiming = PurposeTiming(10 * time.Second)
)

// PurposeDefaults maps each purpose to its default options (applied before user properties).
type PurposeDefaults map[ClientPurpose][]kgo.Opt

// DefaultPurposeDefaults returns the standard profile per purpose.
func DefaultPurposeDefaults() PurposeDefaults {
	return PurposeDefaults{
		PurposeAdmin:    {kgo.RetryTimeout(time.Duration(AdminRetryTimeout))},
		PurposeSampler:  {kgo.FetchMaxWait(time.Duration(SamplerFetchMaxWait))},
		PurposeProducer: {kgo.RequiredAcks(kgo.AllISRAcks())},
		PurposeConsumer: {},
	}
}

// FranzClientFactory builds franz-go clients from seeds, security, purpose defaults and properties.
type FranzClientFactory struct {
	security   *SecurityRegistry
	properties *PropertyCatalog
	purposes   PurposeDefaults
}

// NewFranzClientFactory builds the factory.
func NewFranzClientFactory(security *SecurityRegistry, properties *PropertyCatalog, purposes PurposeDefaults) *FranzClientFactory {
	return &FranzClientFactory{security: security, properties: properties, purposes: purposes}
}

// Kind implements ClientFactory.
func (f *FranzClientFactory) Kind() ClusterKind { return ClusterKafka }

// NewClient implements ClientFactory; it does not dial until the first request.
func (f *FranzClientFactory) NewClient(_ context.Context, spec ConnectionSpec, purpose ClientPurpose) (Client, error) {
	opts, err := f.Options(spec, purpose)
	if err != nil {
		return nil, err
	}
	cl, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("kafka: new client: %w", err)
	}
	return NewFranzClient(cl), nil
}

// Options assembles every option for spec and purpose.
func (f *FranzClientFactory) Options(spec ConnectionSpec, purpose ClientPurpose) ([]kgo.Opt, error) {
	opts := []kgo.Opt{kgo.SeedBrokers(spec.BootstrapServers...)}
	sec, err := f.security.Options(spec)
	if err != nil {
		return nil, err
	}
	opts = append(opts, sec...)
	opts = append(opts, f.purposes[purpose]...)
	props, err := f.properties.Options(spec.Properties)
	if err != nil {
		return nil, err
	}
	return append(opts, props...), nil
}

// FranzClient adapts *kgo.Client to Client.
type FranzClient struct {
	raw   *kgo.Client
	admin Admin
}

// NewFranzClient wraps a franz-go client.
func NewFranzClient(cl *kgo.Client) *FranzClient {
	return &FranzClient{raw: cl, admin: NewFranzAdmin(cl)}
}

// Raw implements Client.
func (c *FranzClient) Raw() *kgo.Client { return c.raw }

// Admin implements Client.
func (c *FranzClient) Admin() Admin { return c.admin }

// Seeds implements Client: the bootstrap servers the client actually dials.
func (c *FranzClient) Seeds() []string {
	seeds, _ := c.raw.OptValue(kgo.SeedBrokers).([]string)
	return seeds
}

// Close implements Client.
func (c *FranzClient) Close() { c.raw.Close() }

// ClientFactoryRegistry is the Manager selecting a factory by the spec's cluster kind.
type ClientFactoryRegistry struct {
	byKind map[ClusterKind]ClientFactory
}

// NewClientFactoryRegistry returns an empty registry.
func NewClientFactoryRegistry() *ClientFactoryRegistry {
	return &ClientFactoryRegistry{byKind: map[ClusterKind]ClientFactory{}}
}

// DefaultClientFactoryRegistry registers the franz-go factory with the default managers and the
// demo factory on the process-wide demo cluster pool.
func DefaultClientFactoryRegistry() *ClientFactoryRegistry {
	r := NewClientFactoryRegistry()
	r.Register(NewFranzClientFactory(DefaultSecurityRegistry(), DefaultPropertyCatalog(), DefaultPurposeDefaults()))
	r.Register(DefaultDemoClientFactory())
	return r
}

// Register adds or replaces the factory of its kind.
func (r *ClientFactoryRegistry) Register(f ClientFactory) {
	r.byKind[f.Kind()] = f
}

// For returns the factory of a kind.
func (r *ClientFactoryRegistry) For(kind ClusterKind) (ClientFactory, error) {
	f, ok := r.byKind[kind]
	if !ok {
		return nil, fmt.Errorf("kafka: no client factory for cluster kind %q", kind)
	}
	return f, nil
}

// NewClient opens a client through the factory of spec's kind.
func (r *ClientFactoryRegistry) NewClient(ctx context.Context, spec ConnectionSpec, purpose ClientPurpose) (Client, error) {
	f, err := r.For(spec.ClusterKindOrDefault())
	if err != nil {
		return nil, err
	}
	return f.NewClient(ctx, spec, purpose)
}
