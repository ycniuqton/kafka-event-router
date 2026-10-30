package lookup

import (
	"context"
	"errors"
	"time"

	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// ErrCircuitOpen is the cause of calls refused by an open breaker.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// GuardedClient wraps a Client with the source's breaker, limiter and metrics.
type GuardedClient struct {
	source   string
	client   Client
	breaker  *CircuitBreaker
	limiter  *Limiter
	observer Observer
}

// NewGuardedClient builds the guard of spec around client.
func NewGuardedClient(spec SourceSpec, client Client, observer Observer) *GuardedClient {
	if observer == nil {
		observer = NoopObserver{}
	}
	onChange := func(s BreakerState) { observer.Breaker(spec.ID, s) }
	openFor := time.Duration(spec.Limits.BreakerOpenMs) * time.Millisecond
	return &GuardedClient{
		source: spec.ID, client: client, observer: observer, limiter: NewLimiter(spec.Limits),
		breaker: NewCircuitBreaker(spec.Limits.BreakerFailures, openFor, nil, onChange),
	}
}

// Fetch calls through breaker and limiter and records the outcome.
func (g *GuardedClient) Fetch(ctx context.Context, req ctxhelpers.SourceRequest) (ctxhelpers.SourceAnswer, error) {
	start := time.Now()
	if !g.breaker.Allow() {
		g.observer.Request(g.source, ResultCircuitOpen, time.Since(start))
		return ctxhelpers.SourceAnswer{}, Failures{}.Transient(ReasonCircuitOpen, ErrCircuitOpen)
	}
	release, err := g.limiter.Acquire(ctx)
	if err != nil {
		g.breaker.Cancel()
		g.observer.Request(g.source, ResultRateLimited, time.Since(start))
		return ctxhelpers.SourceAnswer{}, err
	}
	answer, err := g.client.Fetch(ctx, req)
	release()
	g.breaker.Record((ctxhelpers.SourceErrors{}).IsTransient(err))
	g.observer.Request(g.source, RequestResults{}.Of(answer, err), time.Since(start))
	return answer, err
}

// Probe checks the source without the guard (Test button).
func (g *GuardedClient) Probe(ctx context.Context) error { return g.client.Probe(ctx) }

// Close implements Client.
func (g *GuardedClient) Close() { g.client.Close() }

// RequestResults derives the metric result of a call.
type RequestResults struct{}

// Of returns hit, miss, timeout or error.
func (RequestResults) Of(answer ctxhelpers.SourceAnswer, err error) RequestResult {
	switch {
	case err != nil && errors.Is(err, context.DeadlineExceeded):
		return ResultTimeout
	case err != nil:
		return ResultError
	case answer.Found:
		return ResultHit
	}
	return ResultMiss
}
