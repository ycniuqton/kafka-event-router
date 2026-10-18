// Package producers owns the ref-counted ProducerPool: idempotent acks=all producer clients keyed by
// connection, credential hash and delivery mode; routes with matching keys share one client when
// ProducerSharing is on (P2 default), otherwise every runner owns its clients.
package producers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/retry"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/kafka"
)

// ClientOpener opens a franz-go client for a connection spec.
type ClientOpener interface {
	Open(ctx context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose, extra ...kgo.Opt) (*kgo.Client, error)
}

// Key identifies one pooled producer; Owner is set when sharing is off so every runner gets its own.
type Key struct {
	ConnectionID   string
	CredentialHash string
	Mode           config.DeliveryMode
	Owner          string
}

// PoolLimit bounds a pooled client.
type PoolLimit int

// SharedBufferedRecords lifts franz-go's 10k buffered-record cap on shared clients: every runner already
// bounds its own records by limits.maxInFlight, so one slow route must not block produces of the others.
const SharedBufferedRecords PoolLimit = 1 << 20

// MetricName names the pool's worker metric.
type MetricName string

const MetricProducerClients MetricName = "router_worker_producer_clients"

// LogAttr names the pool's log attributes.
type LogAttr string

const AttrSharing LogAttr = "producer_sharing"

// pooled is one open producer and its reference count.
type pooled struct {
	client *kgo.Client
	refs   int
}

// Lease is a runner's handle on a pooled producer.
type Lease struct {
	key    Key
	Client *kgo.Client
}

// Pool hands out producer clients and closes them when the last lease is released.
type Pool struct {
	mu      sync.Mutex
	opener  ClientOpener
	sharing domain.ProducerSharing
	retry   retry.Policy
	clients map[Key]*pooled
	gauge   prometheus.Gauge
}

// NewPool builds a pool; sharing off (MVP) keys producers by owner as well.
func NewPool(opener ClientOpener, sharing domain.ProducerSharing, policy retry.Policy) *Pool {
	return &Pool{opener: opener, sharing: sharing, retry: policy, clients: map[Key]*pooled{},
		gauge: prometheus.NewGauge(prometheus.GaugeOpts{Name: string(MetricProducerClients)})}
}

// Register exposes the open-client gauge on reg.
func (p *Pool) Register(reg prometheus.Registerer) error {
	return reg.Register(p.gauge)
}

// Sharing returns the pool's sharing mode.
func (p *Pool) Sharing() domain.ProducerSharing { return p.sharing }

// Acquire returns a lease on the producer of spec for owner, opening it on first use.
func (p *Pool) Acquire(ctx context.Context, spec kafka.ConnectionSpec, mode config.DeliveryMode, owner string) (*Lease, error) {
	key := p.keyOf(spec, mode, owner)
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[key]; ok {
		c.refs++
		return &Lease{key: key, Client: c.client}, nil
	}
	cl, err := p.opener.Open(ctx, spec, kafka.PurposeProducer, p.options()...)
	if err != nil {
		return nil, err
	}
	p.clients[key] = &pooled{client: cl, refs: 1}
	p.gauge.Set(float64(len(p.clients)))
	return &Lease{key: key, Client: cl}, nil
}

// Release returns a lease; the client is flushed and closed with the last one.
func (p *Pool) Release(ctx context.Context, l *Lease) {
	p.mu.Lock()
	c, ok := p.clients[l.key]
	if ok {
		c.refs--
	}
	last := ok && c.refs <= 0
	if last {
		delete(p.clients, l.key)
		p.gauge.Set(float64(len(p.clients)))
	}
	p.mu.Unlock()
	if last {
		_ = c.client.Flush(ctx)
		c.client.Close()
	}
}

// Size returns the number of open producers.
func (p *Pool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.clients)
}

// TransactionalOptions are the producer options of a runner-owned transactional client (never shared).
func (p *Pool) TransactionalOptions() []kgo.Opt {
	return []kgo.Opt{
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordPartitioner(DestinationPartitioner{Default: config.PartitioningModeByKey}),
		kgo.RetryBackoffFn(p.retry.BackoffFn()),
	}
}

func (p *Pool) options() []kgo.Opt {
	opts := p.TransactionalOptions()
	if p.sharing == domain.SharingOn {
		opts = append(opts, kgo.MaxBufferedRecords(int(SharedBufferedRecords)))
	}
	return opts
}

func (p *Pool) keyOf(spec kafka.ConnectionSpec, mode config.DeliveryMode, owner string) Key {
	k := Key{ConnectionID: spec.ID, CredentialHash: CredentialHash{}.Of(spec), Mode: mode}
	if p.sharing != domain.SharingOn {
		k.Owner = owner
	}
	return k
}

// CredentialHash fingerprints everything that authenticates a client (never logged, never stored).
type CredentialHash struct{}

// Of returns the hex SHA-256 of seeds, security, auth and secrets of spec.
func (CredentialHash) Of(spec kafka.ConnectionSpec) string {
	raw, _ := json.Marshal(struct {
		Seeds  []string
		Mode   kafka.SecurityMode
		TLS    kafka.TLSSpec
		Auth   kafka.AuthSpec
		Secret kafka.SecretBundle
	}{spec.BootstrapServers, spec.SecurityMode, spec.TLS, spec.Auth, spec.Secret})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
