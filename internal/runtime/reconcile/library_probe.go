package reconcile

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/eventrouter/router/pkg/config"
)

// LibraryProbe wraps the worker's library resolver for one compile and remembers transient load
// failures (control plane unreachable), so they are retried instead of failing the version.
type LibraryProbe struct {
	inner     config.LibraryResolver
	mu        sync.Mutex
	transient error
}

// NewLibraryProbe wraps inner; a nil inner leaves library steps without a resolver.
func NewLibraryProbe(inner config.LibraryResolver) *LibraryProbe {
	return &LibraryProbe{inner: inner}
}

// Resolver returns the resolver to compile with (nil without a library).
func (p *LibraryProbe) Resolver() config.LibraryResolver {
	if p.inner == nil {
		return nil
	}
	return p
}

// Transform implements config.LibraryResolver.
func (p *LibraryProbe) Transform(ctx context.Context, ref string, version int) (config.Steps, error) {
	steps, err := p.inner.Transform(ctx, ref, version)
	return steps, p.observe(err)
}

// LookupTable implements config.LibraryResolver.
func (p *LibraryProbe) LookupTable(ctx context.Context, ref string, version int) (config.LookupRows, error) {
	rows, err := p.inner.LookupTable(ctx, ref, version)
	return rows, p.observe(err)
}

// observe records failures other than a missing pin.
func (p *LibraryProbe) observe(err error) error {
	if err != nil && !errors.Is(err, config.ErrLibraryRefMissing) {
		p.mu.Lock()
		p.transient = err
		p.mu.Unlock()
	}
	return err
}

// Classify returns a retryable error when a library load failed transiently, else a CompileError.
func (p *LibraryProbe) Classify(err error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.transient != nil {
		return fmt.Errorf("worker: library unavailable: %w", p.transient)
	}
	return &CompileError{Cause: err}
}

var _ config.LibraryResolver = (*LibraryProbe)(nil)
