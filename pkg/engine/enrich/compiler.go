package enrich

import (
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/step"
)

// Compiler compiles the source mode of a lookup step (called by recipes.Lookup when `lookup` is set).
type Compiler struct {
	Requests *RequestCompilerManager
	Policies *PolicyManager
	Now      func() time.Time
}

// DefaultCompiler uses every built-in request kind and policy with the wall clock for the cache.
func DefaultCompiler() Compiler {
	return Compiler{Requests: DefaultRequestCompilers(), Policies: DefaultPolicies(), Now: time.Now}
}

// Compile builds the SourceLookup step, recording issues under `/…/lookup`.
func (c Compiler) Compile(sc *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	spec := raw.Lookup
	ptr := sc.Pointer.Token(TokenLookup)
	okID := true
	if spec.SourceID == "" {
		sc.Scope.Fail(config.IssueSchemaInvalid, ptr.Token(TokenSourceID), "sourceId is required")
		okID = false
	}
	request, okR := c.Requests.Compile(RequestCompileCtx{Step: sc, Pointer: ptr.Token(TokenRequest)}, spec.Request)
	mappings, okM := MappingCompiler{}.Compile(sc, ptr.Token(TokenMap), spec.Map)
	miss, okMiss := c.Policies.Miss(sc, ptr.Token(TokenOnMiss), spec.OnMiss)
	failure, okErr := c.Policies.Error(sc, ptr.Token(TokenOnError), spec.OnError)
	s := &SourceLookup{
		Base: step.NewBase(sc, raw), sourceID: spec.SourceID, request: request, mappings: mappings,
		runner: CallRunner{Settings: CallSettingsOf{}.Of(spec)}, miss: miss, failure: failure,
	}
	if settings, on := (CacheSettingsOf{}).Of(spec.Cache); on {
		s.cache = NewAnswerCache(settings, c.Now)
	}
	if okR {
		RequestAccess{}.Record(sc, s)
	}
	return s, okID && okR && okM && okMiss && okErr
}

// RequestAccess feeds the declared field reads of a compiled lookup (its request placeholders, parsed outside
// the compile scope) to the fast-codec field recorder; map targets are already recorded through Scope.Path.
type RequestAccess struct{}

// Record adds every placeholder path of s to the route's projection recorder (no-op without one).
func (RequestAccess) Record(sc *step.StepCompileCtx, s step.FieldAccessor) {
	for _, p := range s.FieldAccess().Reads {
		sc.Scope.Access.Path(p)
	}
}

// CallSettingsOf applies the documented defaults to timeoutMs / retries.
type CallSettingsOf struct{}

// Of returns the call settings of spec.
func (CallSettingsOf) Of(spec *config.LookupSpec) CallSettings {
	s := CallSettings{Timeout: time.Duration(DefaultCallTimeout), Retries: DefaultCallRetries}
	if spec.TimeoutMs != nil && *spec.TimeoutMs > 0 {
		s.Timeout = time.Duration(*spec.TimeoutMs) * time.Millisecond
	}
	if spec.Retries != nil && *spec.Retries >= 0 {
		s.Retries = *spec.Retries
	}
	return s
}
