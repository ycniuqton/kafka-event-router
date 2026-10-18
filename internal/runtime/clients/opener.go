// Package clients opens franz-go clients for route runners from connection specs, adding runner-specific
// options (consumer group, partitioner, retry backoff) on top of the pkg/kafka factory defaults.
package clients

import (
	"context"
	"errors"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/pkg/kafka"
)

// Opener is the base of every cluster-kind client opener.
type Opener interface {
	Kind() kafka.ClusterKind
	Open(ctx context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose, extra ...kgo.Opt) (*kgo.Client, error)
}

// SessionOpener is the capability of cluster kinds that support transactions (exactly_once sources).
type SessionOpener interface {
	Kind() kafka.ClusterKind
	Session(ctx context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose, extra ...kgo.Opt) (*kgo.GroupTransactSession, error)
}

// ErrNoSessions is returned for cluster kinds without transaction support.
var ErrNoSessions = errors.New("cluster kind does not support transactional sessions")

// FranzOpener opens real Kafka clients through the shared franz-go factory options.
type FranzOpener struct {
	Factory *kafka.FranzClientFactory
}

// NewFranzOpener builds the opener with the default security, property and purpose managers.
func NewFranzOpener() FranzOpener {
	return FranzOpener{Factory: kafka.NewFranzClientFactory(kafka.DefaultSecurityRegistry(), kafka.DefaultPropertyCatalog(), kafka.DefaultPurposeDefaults())}
}

// Kind implements Opener.
func (FranzOpener) Kind() kafka.ClusterKind { return kafka.ClusterKafka }

// Open implements Opener; it does not dial until the first request.
func (o FranzOpener) Open(_ context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose, extra ...kgo.Opt) (*kgo.Client, error) {
	opts, err := o.Factory.Options(spec, purpose)
	if err != nil {
		return nil, err
	}
	cl, err := kgo.NewClient(append(opts, extra...)...)
	if err != nil {
		return nil, fmt.Errorf("runtime: open %s client: %w", purpose, err)
	}
	return cl, nil
}

// Session implements Opener: a group consumer that also produces transactionally (exactly_once).
func (o FranzOpener) Session(_ context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose, extra ...kgo.Opt) (*kgo.GroupTransactSession, error) {
	opts, err := o.Factory.Options(spec, purpose)
	if err != nil {
		return nil, err
	}
	s, err := kgo.NewGroupTransactSession(append(opts, extra...)...)
	if err != nil {
		return nil, fmt.Errorf("runtime: open %s transact session: %w", purpose, err)
	}
	return s, nil
}

// OpenerManager selects the opener by the spec's cluster kind.
type OpenerManager struct {
	byKind   map[kafka.ClusterKind]Opener
	sessions map[kafka.ClusterKind]SessionOpener
}

// NewOpenerManager registers the given openers; those that can open transact sessions serve exactly_once.
func NewOpenerManager(openers ...Opener) *OpenerManager {
	m := &OpenerManager{byKind: map[kafka.ClusterKind]Opener{}, sessions: map[kafka.ClusterKind]SessionOpener{}}
	for _, o := range openers {
		m.byKind[o.Kind()] = o
		if s, ok := o.(SessionOpener); ok {
			m.sessions[s.Kind()] = s
		}
	}
	return m
}

// Open opens a client through the opener of spec's kind.
func (m *OpenerManager) Open(ctx context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose, extra ...kgo.Opt) (*kgo.Client, error) {
	o, err := m.opener(spec)
	if err != nil {
		return nil, err
	}
	return o.Open(ctx, spec, purpose, extra...)
}

// Session opens a transact session through the opener of spec's kind.
func (m *OpenerManager) Session(ctx context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose, extra ...kgo.Opt) (*kgo.GroupTransactSession, error) {
	s, ok := m.sessions[spec.ClusterKindOrDefault()]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNoSessions, spec.ClusterKindOrDefault())
	}
	return s.Session(ctx, spec, purpose, extra...)
}

func (m *OpenerManager) opener(spec kafka.ConnectionSpec) (Opener, error) {
	o, ok := m.byKind[spec.ClusterKindOrDefault()]
	if !ok {
		return nil, fmt.Errorf("runtime: no client opener for cluster kind %q", spec.ClusterKindOrDefault())
	}
	return o, nil
}

// Admin opens an admin client and wraps it with pkg/kafka admin operations.
func (m *OpenerManager) Admin(ctx context.Context, spec kafka.ConnectionSpec) (kafka.Client, error) {
	cl, err := m.Open(ctx, spec, kafka.PurposeAdmin)
	if err != nil {
		return nil, err
	}
	return kafka.NewFranzClient(cl), nil
}
