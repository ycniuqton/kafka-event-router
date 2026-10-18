package assist

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// LLMProvider is the base contract of every assistant backend (mock today, real models later).
type LLMProvider interface {
	Kind() ProviderKind
	Complete(ctx context.Context, p Prompt) (Completion, error)
}

// ProviderFactory builds one provider from the assistant settings.
type ProviderFactory func(s Settings) (LLMProvider, error)

// ProviderRegistry maps provider kinds to factories; implementations register themselves in wiring.
type ProviderRegistry struct {
	factories map[ProviderKind]ProviderFactory
}

// NewProviderRegistry returns an empty registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{factories: map[ProviderKind]ProviderFactory{}}
}

// Register adds or replaces the factory of kind.
func (r *ProviderRegistry) Register(kind ProviderKind, f ProviderFactory) {
	r.factories[kind] = f
}

// Kinds lists the registered kinds in order.
func (r *ProviderRegistry) Kinds() []ProviderKind {
	out := make([]ProviderKind, 0, len(r.factories))
	for k := range r.factories {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Build creates the configured provider and wraps it in a ProviderManager; unknown kinds fail.
func (r *ProviderRegistry) Build(s Settings) (*ProviderManager, error) {
	f, ok := r.factories[s.Provider]
	if !ok {
		return nil, fmt.Errorf("assist: unknown provider %q (registered: %v)", s.Provider, r.Kinds())
	}
	p, err := f(s)
	if err != nil {
		return nil, fmt.Errorf("assist: provider %q: %w", s.Provider, err)
	}
	return NewProviderManager(p, s.Limits), nil
}

// ProviderManager is the only way business logic reaches a provider: it caps prompts, applies the
// timeout and observes every call.
type ProviderManager struct {
	provider LLMProvider
	limits   Limits
	metrics  Metrics
	now      func() time.Time
}

// NewProviderManager wraps the active provider (no metrics until WithMetrics).
func NewProviderManager(p LLMProvider, limits Limits) *ProviderManager {
	return &ProviderManager{provider: p, limits: limits, metrics: NoMetrics{}, now: time.Now}
}

// WithMetrics makes the manager observe calls on m.
func (m *ProviderManager) WithMetrics(metrics Metrics) *ProviderManager {
	m.metrics = metrics
	return m
}

// Metrics returns the observer (shared with provider-free assistant tasks).
func (m *ProviderManager) Metrics() Metrics { return m.metrics }

// Kind returns the active provider kind.
func (m *ProviderManager) Kind() ProviderKind { return m.provider.Kind() }

// Limits returns the request limits.
func (m *ProviderManager) Limits() Limits { return m.limits }

// Complete trims the prompt to the limits and calls the provider; failures become ErrUnavailable.
func (m *ProviderManager) Complete(ctx context.Context, p Prompt) (Completion, error) {
	ctx, cancel := context.WithTimeout(ctx, m.limits.Timeout)
	defer cancel()
	start := m.now()
	c, err := m.provider.Complete(ctx, m.capped(p))
	if err == nil {
		err = ctx.Err()
	}
	result := ResultOK
	if err != nil {
		result = ResultError
	}
	m.metrics.Observe(p.Task, m.provider.Kind(), result, m.now().Sub(start))
	if err != nil {
		return Completion{}, ErrUnavailable.Wrap(err)
	}
	return c, nil
}

// capped keeps at most MaxSamples samples, each no larger than MaxSampleBytes.
func (m *ProviderManager) capped(p Prompt) Prompt {
	kept := p.Samples[:0:0]
	for _, s := range p.Samples {
		if len(kept) >= m.limits.MaxSamples {
			break
		}
		if len(s) <= m.limits.MaxSampleBytes {
			kept = append(kept, s)
		}
	}
	p.Samples = kept
	return p
}
