package kafka

import (
	"context"
	"fmt"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
)

// DemoClusterKey identifies one demo cluster of a process (the connection id; drafts share one).
type DemoClusterKey string

const DemoDraftKey DemoClusterKey = "draft"

// DemoClusterKeyOf returns the pool key of a spec.
func DemoClusterKeyOf(spec ConnectionSpec) DemoClusterKey {
	if spec.ID == "" {
		return DemoDraftKey
	}
	return DemoClusterKey(spec.ID)
}

// DemoClusterPool owns the in-process demo clusters, one per demo connection, started on first use.
// Each process (api, preview, worker) hosts its own clusters fed by the same deterministic generator.
type DemoClusterPool struct {
	mu       sync.Mutex
	clusters map[DemoClusterKey]*DemoCluster
	settings DemoSettings
}

// NewDemoClusterPool builds an empty pool.
func NewDemoClusterPool(settings DemoSettings) *DemoClusterPool {
	return &DemoClusterPool{clusters: map[DemoClusterKey]*DemoCluster{}, settings: settings}
}

var (
	sharedDemoPoolOnce sync.Once
	sharedDemoPool     *DemoClusterPool
)

// SharedDemoClusterPool returns the process-wide pool, so every factory of a process sees the same clusters.
func SharedDemoClusterPool() *DemoClusterPool {
	sharedDemoPoolOnce.Do(func() { sharedDemoPool = NewDemoClusterPool(DefaultDemoSettings()) })
	return sharedDemoPool
}

// Acquire returns the running cluster of key, starting and seeding it when absent.
func (p *DemoClusterPool) Acquire(ctx context.Context, key DemoClusterKey) (*DemoCluster, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clusters[key]; ok {
		return c, nil
	}
	c, err := StartDemoCluster(ctx, p.settings)
	if err != nil {
		return nil, err
	}
	p.clusters[key] = c
	return c, nil
}

// Release stops and forgets the cluster of key (after its connection is deleted).
func (p *DemoClusterPool) Release(key DemoClusterKey) {
	p.mu.Lock()
	c, ok := p.clusters[key]
	delete(p.clusters, key)
	p.mu.Unlock()
	if ok {
		c.Close()
	}
}

// Close stops every cluster.
func (p *DemoClusterPool) Close() {
	p.mu.Lock()
	all := p.clusters
	p.clusters = map[DemoClusterKey]*DemoCluster{}
	p.mu.Unlock()
	for _, c := range all {
		c.Close()
	}
}

// DemoClientFactory opens clients on the in-process demo cluster of a `kind=demo` connection; the
// spec's bootstrap servers, security and properties are ignored. It also satisfies the runtime's
// cluster-kind Opener contract through Open and Session.
type DemoClientFactory struct {
	pool     *DemoClusterPool
	purposes PurposeDefaults
}

// NewDemoClientFactory builds the factory over a pool.
func NewDemoClientFactory(pool *DemoClusterPool, purposes PurposeDefaults) *DemoClientFactory {
	return &DemoClientFactory{pool: pool, purposes: purposes}
}

// DefaultDemoClientFactory uses the process-wide pool and the standard purpose defaults.
func DefaultDemoClientFactory() *DemoClientFactory {
	return NewDemoClientFactory(SharedDemoClusterPool(), DefaultPurposeDefaults())
}

// Kind implements ClientFactory.
func (f *DemoClientFactory) Kind() ClusterKind { return ClusterDemo }

// Pool returns the cluster pool (to release a deleted connection's cluster).
func (f *DemoClientFactory) Pool() *DemoClusterPool { return f.pool }

// NewClient implements ClientFactory.
func (f *DemoClientFactory) NewClient(ctx context.Context, spec ConnectionSpec, purpose ClientPurpose) (Client, error) {
	cl, err := f.Open(ctx, spec, purpose)
	if err != nil {
		return nil, err
	}
	return NewFranzClient(cl), nil
}

// Open returns a raw franz-go client on the demo cluster with purpose defaults plus extra options.
func (f *DemoClientFactory) Open(ctx context.Context, spec ConnectionSpec, purpose ClientPurpose, extra ...kgo.Opt) (*kgo.Client, error) {
	opts, err := f.Options(ctx, spec, purpose)
	if err != nil {
		return nil, err
	}
	cl, err := kgo.NewClient(append(opts, extra...)...)
	if err != nil {
		return nil, fmt.Errorf("kafka: new demo client: %w", err)
	}
	return cl, nil
}

// Session returns a transactional group session on the demo cluster (route runners in exactly_once mode).
func (f *DemoClientFactory) Session(ctx context.Context, spec ConnectionSpec, purpose ClientPurpose, extra ...kgo.Opt) (*kgo.GroupTransactSession, error) {
	opts, err := f.Options(ctx, spec, purpose)
	if err != nil {
		return nil, err
	}
	s, err := kgo.NewGroupTransactSession(append(opts, extra...)...)
	if err != nil {
		return nil, fmt.Errorf("kafka: new demo transact session: %w", err)
	}
	return s, nil
}

// Options returns the seeds of the spec's demo cluster and the purpose defaults. Producers may create
// topics: every process has its own in-memory cluster, so topics created elsewhere (destination, DLQ) are missing.
func (f *DemoClientFactory) Options(ctx context.Context, spec ConnectionSpec, purpose ClientPurpose) ([]kgo.Opt, error) {
	cluster, err := f.pool.Acquire(ctx, DemoClusterKeyOf(spec))
	if err != nil {
		return nil, err
	}
	return append([]kgo.Opt{kgo.SeedBrokers(cluster.Seeds()...), kgo.AllowAutoTopicCreation()}, f.purposes[purpose]...), nil
}
