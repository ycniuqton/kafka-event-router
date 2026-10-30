package enginetest

import (
	"context"
	"errors"
	"sync"

	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// FakeAnswer is a scripted answer of the FakeSourceGateway (Err set = the call fails).
type FakeAnswer struct {
	Found bool
	Value any
	Err   error
}

// FakeSourceGateway answers lookup calls from a function and records every call and observation.
type FakeSourceGateway struct {
	Answer func(call ctxhelpers.SourceCall) FakeAnswer
	mu     sync.Mutex
	calls  []ctxhelpers.SourceCall
	events map[ctxhelpers.SourceEvent]int
}

// NewFakeSourceGateway builds a gateway answering with fn.
func NewFakeSourceGateway(fn func(call ctxhelpers.SourceCall) FakeAnswer) *FakeSourceGateway {
	return &FakeSourceGateway{Answer: fn, events: map[ctxhelpers.SourceEvent]int{}}
}

// Fetch implements ctxhelpers.SourceGateway; it honours ctx cancellation before answering.
func (g *FakeSourceGateway) Fetch(ctx context.Context, call ctxhelpers.SourceCall) (ctxhelpers.SourceAnswer, error) {
	g.mu.Lock()
	g.calls = append(g.calls, call)
	g.mu.Unlock()
	a := g.Answer(call)
	if err := ctx.Err(); err != nil {
		return ctxhelpers.SourceAnswer{}, err
	}
	if a.Err != nil {
		return ctxhelpers.SourceAnswer{}, a.Err
	}
	return ctxhelpers.SourceAnswer{Found: a.Found, Value: a.Value, Mode: ctxhelpers.AnswerLive}, nil
}

// Observe implements ctxhelpers.SourceGateway.
func (g *FakeSourceGateway) Observe(_ string, ev ctxhelpers.SourceEvent) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.events[ev]++
}

// Calls returns the recorded calls.
func (g *FakeSourceGateway) Calls() []ctxhelpers.SourceCall {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]ctxhelpers.SourceCall(nil), g.calls...)
}

// GoldenSource names the deterministic lookup sources of golden routes.
type GoldenSource string

const (
	SourceCRM   GoldenSource = "golden-crm"
	SourceFlags GoldenSource = "golden-flags"
)

// goldenCustomers are the answers of golden-crm (`/customers/<id>`); c9 is down (503).
var goldenCustomers = map[string]map[string]any{
	"/customers/c1": {"tier": "gold", "region": "eu"},
	"/customers/c2": {"tier": "silver", "region": "us"},
}

// errGoldenDown is the cause of the golden CRM outage.
var errGoldenDown = errors.New("503 Service Unavailable")

// GoldenSources answers golden-crm (http) and golden-flags (redis get `flag:<name>` → "on").
func GoldenSources() *FakeSourceGateway {
	return NewFakeSourceGateway(func(call ctxhelpers.SourceCall) FakeAnswer {
		switch GoldenSource(call.SourceID) {
		case SourceCRM:
			return goldenCRM{}.answer(call.Request)
		case SourceFlags:
			return FakeAnswer{Found: call.Request.Key == "flag:beta", Value: "on"}
		}
		return FakeAnswer{Err: &ctxhelpers.SourceError{Reason: "unknown lookup source"}}
	})
}

// goldenCRM is the scripted CRM API.
type goldenCRM struct{}

func (goldenCRM) answer(req ctxhelpers.SourceRequest) FakeAnswer {
	if req.Path == "/customers/c9" {
		return FakeAnswer{Err: &ctxhelpers.SourceError{Transient: true, Reason: "source unavailable", Cause: errGoldenDown}}
	}
	row, ok := goldenCustomers[req.Path]
	return FakeAnswer{Found: ok, Value: row}
}

// Observed returns how often ev was reported.
func (g *FakeSourceGateway) Observed(ev ctxhelpers.SourceEvent) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.events[ev]
}
