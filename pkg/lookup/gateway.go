package lookup

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// SpecProvider resolves a source with its secrets (remote control plane in workers, the service in the API).
type SpecProvider interface {
	Spec(ctx context.Context, id string) (SourceSpec, error)
}

// ErrSourceNotFound is returned by providers for unknown ids (a permanent failure).
var ErrSourceNotFound = errors.New("lookup source not found")

// GatewayTiming bounds how long a resolved spec is trusted and how long a replaced client may finish calls.
type GatewayTiming time.Duration

const (
	DefaultSpecTTL    GatewayTiming = GatewayTiming(time.Minute)
	ReplacedCloseWait GatewayTiming = GatewayTiming(30 * time.Second)
)

// GatewayDeps are the collaborators of a Gateway.
type GatewayDeps struct {
	Specs     SpecProvider
	Factories *FactoryRegistry
	Observer  Observer
	SpecTTL   time.Duration
	Now       func() time.Time
}

// Gateway implements ctxhelpers.SourceGateway: one guarded client per source, rebuilt when the spec version changes.
type Gateway struct {
	d     GatewayDeps
	mu    sync.Mutex
	slots map[string]*sourceSlot
}

// sourceSlot serialises loading of one source.
type sourceSlot struct {
	mu       sync.Mutex
	client   *GuardedClient
	spec     SourceSpec
	loadedAt time.Time
}

// NewGateway builds a gateway with defaults for missing deps.
func NewGateway(d GatewayDeps) *Gateway {
	if d.Observer == nil {
		d.Observer = NoopObserver{}
	}
	if d.SpecTTL <= 0 {
		d.SpecTTL = time.Duration(DefaultSpecTTL)
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Factories == nil {
		d.Factories = DefaultFactories(nil)
	}
	return &Gateway{d: d, slots: map[string]*sourceSlot{}}
}

// Fetch implements ctxhelpers.SourceGateway.
func (g *Gateway) Fetch(ctx context.Context, call ctxhelpers.SourceCall) (ctxhelpers.SourceAnswer, error) {
	client, spec, err := g.client(ctx, call.SourceID)
	if err != nil {
		return ctxhelpers.SourceAnswer{}, err
	}
	if spec.TeamID != "" && call.TeamID != "" && spec.TeamID != call.TeamID {
		return ctxhelpers.SourceAnswer{}, Failures{}.Permanent(ReasonForbidden, fmt.Errorf("source %q", spec.Name))
	}
	if RequestKindOf[spec.Kind] != call.Kind {
		return ctxhelpers.SourceAnswer{}, Failures{}.Permanent(ReasonKindMismatch, fmt.Errorf("%s request to %s source %q", call.Kind, spec.Kind, spec.Name))
	}
	answer, err := client.Fetch(ctx, call.Request)
	answer.Mode = ctxhelpers.AnswerLive
	return answer, err
}

// Observe implements ctxhelpers.SourceGateway.
func (g *Gateway) Observe(sourceID string, ev ctxhelpers.SourceEvent) {
	g.d.Observer.Cache(sourceID, ev)
}

// Close releases every client.
func (g *Gateway) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for id, s := range g.slots {
		if s.client != nil {
			s.client.Close()
		}
		delete(g.slots, id)
	}
}

func (g *Gateway) slot(id string) *sourceSlot {
	g.mu.Lock()
	defer g.mu.Unlock()
	s, ok := g.slots[id]
	if !ok {
		s = &sourceSlot{}
		g.slots[id] = s
	}
	return s
}

// client returns the source's guarded client, refreshing the spec after SpecTTL (a stale client is kept when
// the refresh fails, so a control-plane blip does not fail lookups).
func (g *Gateway) client(ctx context.Context, id string) (*GuardedClient, SourceSpec, error) {
	s := g.slot(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil && g.d.Now().Sub(s.loadedAt) < g.d.SpecTTL {
		return s.client, s.spec, nil
	}
	spec, err := g.d.Specs.Spec(ctx, id)
	if err != nil && s.client != nil && !errors.Is(err, ErrSourceNotFound) {
		return s.client, s.spec, nil
	}
	if err != nil {
		return nil, SourceSpec{}, SpecErrors{}.Of(err)
	}
	if err := g.install(s, spec); err != nil {
		return nil, SourceSpec{}, err
	}
	return s.client, s.spec, nil
}

// install keeps the client when the version is unchanged, else builds a new one and retires the old one.
func (g *Gateway) install(s *sourceSlot, spec SourceSpec) error {
	s.loadedAt = g.d.Now()
	if s.client != nil && s.spec.Version == spec.Version {
		return nil
	}
	c, err := g.d.Factories.New(spec)
	if err != nil {
		return Failures{}.Permanent(ReasonRejected, err)
	}
	if old := s.client; old != nil {
		time.AfterFunc(time.Duration(ReplacedCloseWait), old.Close)
	}
	s.client, s.spec = NewGuardedClient(spec, c, g.d.Observer), spec
	return nil
}

// SpecErrors classifies spec resolution failures.
type SpecErrors struct{}

// Of returns a permanent error for unknown sources, else transient (control plane unreachable).
func (SpecErrors) Of(err error) error {
	if errors.Is(err, ErrSourceNotFound) {
		return Failures{}.Permanent(ReasonUnknown, err)
	}
	return Failures{}.Transient(ReasonUnavailable, err)
}
