package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/step"
)

// CallDefault holds the call defaults of the lookup document.
type CallDefault time.Duration

const (
	DefaultCallTimeout CallDefault = CallDefault(500 * time.Millisecond)
	DefaultCallRetries             = 1
)

// CallSettings bounds the calls of one step.
type CallSettings struct {
	Timeout time.Duration
	Retries int
}

// LookupResult names the outcome written to the step log line.
type LookupResult string

const (
	ResultHit   LookupResult = "hit"
	ResultMiss  LookupResult = "miss"
	ResultError LookupResult = "error"
)

// LookupLog formats the per-call log line the Studio parses (`lookup: mode=live source=… result=hit ms=12`).
type LookupLog struct{}

// LookupLogPrefix starts every lookup log line.
const LookupLogPrefix = "lookup:"

// Line renders one call.
func (LookupLog) Line(mode ctxhelpers.AnswerMode, source string, result LookupResult, d time.Duration) string {
	return fmt.Sprintf("%s mode=%s source=%s result=%s ms=%d", LookupLogPrefix, mode, source, result, d.Milliseconds())
}

// ErrTimeout is the cause of a call that exceeded its timeout.
var ErrTimeout = errors.New("lookup call timed out")

// CallRunner calls the gateway with a timeout per attempt and retries transient failures.
type CallRunner struct {
	Settings CallSettings
}

// Run returns the first answer, or the last error once retries are exhausted or the error is permanent.
func (r CallRunner) Run(ctx context.Context, gw ctxhelpers.SourceGateway, call ctxhelpers.SourceCall) (ctxhelpers.SourceAnswer, error) {
	var last error
	for attempt := 0; attempt <= r.Settings.Retries; attempt++ {
		answer, err := r.once(ctx, gw, call)
		if err == nil {
			return answer, nil
		}
		last = err
		if !(ctxhelpers.SourceErrors{}).IsTransient(err) || ctx.Err() != nil {
			break
		}
	}
	return ctxhelpers.SourceAnswer{}, last
}

func (r CallRunner) once(ctx context.Context, gw ctxhelpers.SourceGateway, call ctxhelpers.SourceCall) (ctxhelpers.SourceAnswer, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, r.Settings.Timeout)
	defer cancel()
	answer, err := gw.Fetch(attemptCtx, call)
	if err != nil && attemptCtx.Err() != nil && ctx.Err() == nil {
		return answer, &ctxhelpers.SourceError{Transient: true, Reason: ErrTimeout.Error(), Cause: err}
	}
	return answer, err
}

// CacheKey identifies a rendered request.
type CacheKey struct{}

// Of renders kind + canonical JSON of the request (maps are key-sorted by encoding/json).
func (CacheKey) Of(call ctxhelpers.SourceCall) string {
	b, err := json.Marshal(call.Request)
	if err != nil {
		return ""
	}
	return string(call.Kind) + string(b)
}

// SourceLookup is a compiled lookup step in source mode.
type SourceLookup struct {
	step.Base
	sourceID string
	request  RequestBuilder
	mappings Mappings
	runner   CallRunner
	miss     MissPolicy
	failure  ErrorPolicy
	cache    *AnswerCache
}

// Apply renders the request, answers from the cache or the gateway and maps or applies the policies.
func (s *SourceLookup) Apply(sc *step.StepContext, in engine.Event) ([]engine.Event, error) {
	req, ok := s.request.Render(&in)
	if !ok {
		return s.miss.Apply(s.input(in, nil))
	}
	call := ctxhelpers.SourceCall{SourceID: s.sourceID, StepID: s.ID(), TeamID: sc.Meta.TeamID, Kind: s.request.Kind(), Request: req}
	start := time.Now()
	answer, err := s.answer(sc, call)
	if err != nil {
		sc.Log(LookupLog{}.Line(ctxhelpers.AnswerLive, s.sourceID, ResultError, time.Since(start)))
		return s.failure.Apply(s.input(in, err))
	}
	if !answer.Found {
		sc.Log(LookupLog{}.Line(answer.Mode, s.sourceID, ResultMiss, time.Since(start)))
		return s.miss.Apply(s.input(in, nil))
	}
	sc.Log(LookupLog{}.Line(answer.Mode, s.sourceID, ResultHit, time.Since(start)))
	if err := s.mappings.Apply(&in, step.NumberNormalizer{}.Of(answer.Value)); err != nil {
		return nil, err
	}
	return step.One(in), nil
}

// answer serves from the cache, else calls the gateway and caches the outcome (errors are never cached).
func (s *SourceLookup) answer(sc *step.StepContext, call ctxhelpers.SourceCall) (ctxhelpers.SourceAnswer, error) {
	gw := sc.Helpers.Sources
	if gw == nil {
		gw = ctxhelpers.NoSourceGateway{}
	}
	if s.cache == nil {
		return s.runner.Run(sc.Ctx, gw, call)
	}
	key := CacheKey{}.Of(call)
	if hit, ok := s.cache.Get(key); ok {
		gw.Observe(s.sourceID, ctxhelpers.EventCacheHit)
		hit.Mode = ctxhelpers.AnswerCache
		return hit, nil
	}
	gw.Observe(s.sourceID, ctxhelpers.EventCacheMiss)
	answer, err := s.runner.Run(sc.Ctx, gw, call)
	if err == nil && answer.Mode != ctxhelpers.AnswerMock {
		s.cache.Put(key, answer)
	}
	return answer, err
}

func (s *SourceLookup) input(ev engine.Event, cause error) PolicyInput {
	return PolicyInput{Event: ev, Mappings: s.mappings, Source: s.sourceID, Cause: cause}
}

// FieldAccess implements step.FieldAccessor: template placeholders are read, map targets written.
func (s *SourceLookup) FieldAccess() step.FieldAccess {
	return step.FieldAccess{Reads: s.request.Fields(), Writes: s.mappings.Targets()}
}

// SourceID returns the referenced lookup source (usage, builder summaries).
func (s *SourceLookup) SourceID() string { return s.sourceID }
