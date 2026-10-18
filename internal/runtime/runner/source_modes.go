package runner

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/kafka"
)

// SourceMode is the base of every way to open a route's source consumer; one per delivery mode.
type SourceMode interface {
	Mode() config.DeliveryMode
	Open(ctx context.Context, opener ClientOpener, v CompiledVersion, opts []kgo.Opt) (*SourceConsumer, error)
}

// GroupSourceMode opens a plain group consumer (at_least_once).
type GroupSourceMode struct{}

func (GroupSourceMode) Mode() config.DeliveryMode { return config.DeliveryModeAtLeastOnce }

// Open implements SourceMode.
func (GroupSourceMode) Open(ctx context.Context, opener ClientOpener, v CompiledVersion, opts []kgo.Opt) (*SourceConsumer, error) {
	cl, err := opener.Open(ctx, v.Connections[v.Config.Source.ConnectionID], kafka.PurposeConsumer, opts...)
	if err != nil {
		return nil, err
	}
	return &SourceConsumer{client: cl}, nil
}

// TransactSourceMode opens a group transact session (exactly_once): it reads committed records only,
// requires stable offsets and produces with the runner's own transactional id.
type TransactSourceMode struct {
	Producer []kgo.Opt
}

func (TransactSourceMode) Mode() config.DeliveryMode { return config.DeliveryModeExactlyOnce }

// Open implements SourceMode.
func (m TransactSourceMode) Open(ctx context.Context, opener ClientOpener, v CompiledVersion, opts []kgo.Opt) (*SourceConsumer, error) {
	txn := []kgo.Opt{
		kgo.TransactionalID(domain.TransactionalIDs{}.New(v.Ref.RouteID)),
		kgo.FetchIsolationLevel(kgo.ReadCommitted()),
		kgo.RequireStableFetchOffsets(),
	}
	all := append(append(append([]kgo.Opt{}, opts...), m.Producer...), txn...)
	s, err := opener.Session(ctx, v.Connections[v.Config.Source.ConnectionID], kafka.PurposeConsumer, all...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", delivery.ErrNoSession, err)
	}
	return &SourceConsumer{client: s.Client(), session: s}, nil
}

// SourceModes selects the source mode by the route's delivery mode.
type SourceModes struct {
	byMode map[config.DeliveryMode]SourceMode
}

// NewSourceModes registers both modes; producer are the options transactional outputs are produced with.
func NewSourceModes(producer []kgo.Opt) SourceModes {
	m := SourceModes{byMode: map[config.DeliveryMode]SourceMode{}}
	for _, s := range []SourceMode{GroupSourceMode{}, TransactSourceMode{Producer: producer}} {
		m.byMode[s.Mode()] = s
	}
	return m
}

// For returns the source mode of mode.
func (m SourceModes) For(mode config.DeliveryMode) (SourceMode, error) {
	s, ok := m.byMode[mode]
	if !ok {
		return nil, fmt.Errorf("%w: %s", delivery.ErrModeUnavailable, mode)
	}
	return s, nil
}
