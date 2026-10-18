// Package delivery holds the delivery strategies (one per config.DeliveryMode): how outputs are produced
// and how source offsets are committed: at_least_once (MVP) and exactly_once (P3, one transaction per batch).
package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/producers"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/kafka"
)

// Outgoing is one encoded record bound for a destination connection; Branch is the route branch
// that produced it (empty for dead letters).
type Outgoing struct {
	Record       engine.RawRecord
	ConnectionID string
	Partitioning config.PartitioningMode
	Branch       engine.BranchID
}

// Mark is a committable source position (next offset to consume).
type Mark struct {
	Topic     string
	Partition int32
	Offset    int64
}

// AckFunc receives the outcome of one produce.
type AckFunc func(err error)

// BatchMode is how a strategy groups polled records: pipelined across polls, or one transaction per poll.
type BatchMode string

const (
	BatchPipelined     BatchMode = "pipelined"
	BatchTransactional BatchMode = "transactional"
)

// Strategy is the base of every delivery mode.
type Strategy interface {
	Mode() config.DeliveryMode
	Batching() BatchMode
	Produce(ctx context.Context, out Outgoing, onAck AckFunc)
	Commit(ctx context.Context, marks []Mark) error
	BeginBatch(ctx context.Context) error
	EndBatch(ctx context.Context, commit bool) (committed bool, err error)
	Close(ctx context.Context) error
}

// Deps are what a strategy is built from: the runner's consumer (a transact session for exactly_once),
// the producer pool, the specs and the source connection; Abort tells the runner its open batch must abort.
type Deps struct {
	Consumer    *kgo.Client
	Session     *kgo.GroupTransactSession
	Producers   *producers.Pool
	Connections map[string]kafka.ConnectionSpec
	Source      string
	Owner       string
	Abort       func()
}

// Factory builds the strategy of one mode for one runner.
type Factory interface {
	Mode() config.DeliveryMode
	New(ctx context.Context, d Deps) (Strategy, error)
}

// ErrModeUnavailable is returned when the route asks for a delivery mode this build does not provide.
var ErrModeUnavailable = errors.New("delivery mode is not available")

// ErrUnknownConnection is returned when an output names a connection the runner did not resolve.
var ErrUnknownConnection = errors.New("output connection was not resolved for this runner")

// Manager selects the strategy factory by the route's delivery mode.
type Manager struct {
	factories map[config.DeliveryMode]Factory
}

// NewManager registers factories.
func NewManager(factories ...Factory) *Manager {
	m := &Manager{factories: map[config.DeliveryMode]Factory{}}
	for _, f := range factories {
		m.factories[f.Mode()] = f
	}
	return m
}

// NewDefaultManager registers every delivery strategy.
func NewDefaultManager() *Manager {
	return NewManager(AtLeastOnceFactory{}, ExactlyOnceFactory{})
}

// For builds the strategy of mode for one runner.
func (m *Manager) For(ctx context.Context, mode config.DeliveryMode, d Deps) (Strategy, error) {
	f, ok := m.factories[mode]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrModeUnavailable, mode)
	}
	return f.New(ctx, d)
}

// DeliveryModes reads the delivery mode of routes.
type DeliveryModes struct{}

// Of returns the route's delivery mode (absent = at_least_once).
func (DeliveryModes) Of(cfg *config.RouteConfig) config.DeliveryMode {
	if cfg.Delivery == nil || cfg.Delivery.Mode == "" {
		return config.DeliveryModeAtLeastOnce
	}
	return cfg.Delivery.Mode
}
