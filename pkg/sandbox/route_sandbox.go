package sandbox

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eventrouter/router/pkg/sandbox/boundary"
	"github.com/eventrouter/router/pkg/sandbox/determinism"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// moduleBuilder compiles a source and pools its instances for one route version.
type moduleBuilder interface {
	build(ctx context.Context, scope RouteScope, src Source) (*pooledModule, error)
}

// routeSandbox owns the pooled modules of one route version; step paths of every branch alias
// the module of their content key, so identical steps share one pool.
type routeSandbox struct {
	scope     RouteScope
	builder   moduleBuilder
	observer  Observer
	onClose   func(*routeSandbox)
	mu        sync.Mutex
	modules   map[ModuleKey]*pooledModule
	paths     map[string]*pooledModule
	closed    bool
	closeOnce sync.Once
}

func newRouteSandbox(scope RouteScope, builder moduleBuilder, observer Observer, onClose func(*routeSandbox)) *routeSandbox {
	return &routeSandbox{scope: scope, builder: builder, observer: observer, onClose: onClose,
		modules: map[ModuleKey]*pooledModule{}, paths: map[string]*pooledModule{}}
}

// Attach compiles src once per content key and maps its step path to the shared module.
func (r *routeSandbox) Attach(ctx context.Context, src Source) (Module, error) {
	key := ModuleKeyOf(src)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, NewErrorClassifier().New(ErrorInternal, string(MsgPoolClosed))
	}
	if mod, ok := r.modules[key]; ok {
		r.paths[src.StepPath] = mod
		return mod, nil
	}
	mod, err := r.builder.build(ctx, r.scope, src)
	if err != nil {
		return nil, err
	}
	mod.route = r
	r.modules[key], r.paths[src.StepPath] = mod, mod
	return mod, nil
}

func (r *routeSandbox) Module(stepPath string) (Module, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.paths[stepPath]
	return m, ok
}

func (r *routeSandbox) Stats() []PoolStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]PoolStats, 0, len(r.modules))
	for _, m := range r.modules {
		out = append(out, m.pool.Stats())
	}
	return out
}

// Usage sums the pools of this route version.
func (r *routeSandbox) Usage() RouteUsage {
	u := RouteUsage{RouteID: r.scope.RouteID, VersionID: r.scope.VersionID}
	for _, st := range r.Stats() {
		u = u.Add(st)
	}
	return u
}

// observePools reports the summed pool gauges of the route version.
func (r *routeSandbox) observePools() {
	u := r.Usage()
	r.observer.PoolObserved(r.scope, PoolStats{Size: u.Instances, MemoryBytes: u.MemoryBytes, Calls: u.Calls, CPUTime: u.CPUTime})
}

// Close drains every pool once; in-flight calls finish or hit their deadline.
func (r *routeSandbox) Close(ctx context.Context) error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		steps := make([]string, 0, len(r.paths))
		for path := range r.paths {
			steps = append(steps, path)
		}
		modules := r.modules
		r.mu.Unlock()
		for _, m := range modules {
			m.close(ctx)
		}
		r.observer.ScopeClosed(r.scope, steps)
		if r.onClose != nil {
			r.onClose(r)
		}
	})
	return nil
}

// pooledModule runs one step's Program through its pool.
type pooledModule struct {
	scope      RouteScope
	program    Program
	pool       Pool
	marshaller boundary.Marshaller
	classifier ErrorClassifier
	observer   Observer
	policy     determinism.Policy
	route      *routeSandbox
	lease      programLease
	poolSize   atomic.Int64
}

// programLease is the module's reference on its cached program.
type programLease struct {
	cache ProgramCache
	key   ProgramKey
}

// close drains the pool and returns the program reference.
func (m *pooledModule) close(ctx context.Context) {
	_ = m.pool.Close(ctx)
	if m.lease.cache != nil {
		m.lease.cache.Release(m.lease.key)
	}
}

// Call marshals the event, runs it on a pooled instance and validates the result.
func (m *pooledModule) Call(ctx context.Context, inv Invocation) (Result, error) {
	start := time.Now()
	call, err := m.prepare(ctx, inv)
	if err != nil {
		return m.done(start, Result{}, m.classifier.Classify(err))
	}
	inst, err := m.pool.Acquire(ctx)
	if err != nil {
		return m.done(start, Result{}, m.classifier.Classify(err))
	}
	invokeStart := time.Now()
	reply, err := inst.Invoke(ctx, call)
	cpu := time.Since(invokeStart)
	m.pool.Charge(cpu)
	memory := inst.MemoryBytes()
	m.settle(ctx, inst, err)
	res, err := m.decode(reply, err)
	res.CPUTime, res.MemoryBytes, res.Seed, res.Determinism = cpu, memory, call.Seed, determinism.VersionCurrent
	res.Logs, res.LogsTruncated = call.Logs.Lines(), call.Logs.Truncated()
	return m.done(start, res, err)
}

// prepare builds the guest call: marshalled event, frozen clock, seed and log buffer.
func (m *pooledModule) prepare(ctx context.Context, inv Invocation) (GuestCall, error) {
	seed, now, err := m.determinismOf(ctx, inv)
	if err != nil {
		return GuestCall{}, err
	}
	input, err := m.marshaller.ToGuest(inv.Event)
	if err != nil {
		return GuestCall{}, m.classifier.New(ErrorInternal, err.Error())
	}
	logCap := int64(0)
	if m.scope.Mode.KeepsLogs() {
		logCap = m.program.Limits().MaxLogBytes
	}
	return GuestCall{
		Input:     input,
		Predicate: m.program.Source().Entry == EntryPredicate,
		Clock:     determinism.Freeze(now),
		Seed:      seed,
		Logs:      host.NewLogBuffer(logCap),
	}, nil
}

// determinismOf picks seed and now: a test-mode pin replays a saved case (stale versions fail),
// otherwise the processing policy (same for original and replay passes) or SeedOverride applies.
func (m *pooledModule) determinismOf(ctx context.Context, inv Invocation) (uint64, time.Time, error) {
	if pin, ok := m.pinOf(ctx, inv); ok {
		if !pin.Compatible() {
			return 0, time.Time{}, m.classifier.New(ErrorDeterminism, MsgDeterminismStale.Format(pin.Version, determinism.VersionCurrent))
		}
		return pin.Seed, pin.Now, nil
	}
	ref := determinism.EventRef{RouteID: m.scope.RouteID, Topic: inv.Event.Topic, Partition: inv.Event.Partition, Offset: inv.Event.Offset}
	in := m.policy.Inputs(ref, determinism.PassContext{}.From(ctx), inv.Now)
	if inv.SeedOverride != nil {
		in.Seed = *inv.SeedOverride
	}
	return in.Seed, in.Now, nil
}

// pinOf returns the invocation pin, else the context pin; pins only apply in test mode.
func (m *pooledModule) pinOf(ctx context.Context, inv Invocation) (determinism.Pin, bool) {
	if m.scope.Mode != ScopeTest {
		return determinism.Pin{}, false
	}
	if inv.Pin != nil {
		return *inv.Pin, true
	}
	return determinism.PinContext{}.From(ctx)
}

// settle returns the instance to the pool or discards it per the error policy.
func (m *pooledModule) settle(ctx context.Context, inst Instance, err error) {
	action := PoolRelease
	if err != nil {
		action = m.classifier.Policy(m.classifier.Classify(err).Kind).Action
	}
	if action == PoolDiscard || !inst.Reusable() {
		m.pool.Discard(ctx, inst)
		return
	}
	m.pool.Release(inst)
}

// decode turns the guest reply (or call error) into a Result according to the entry contract.
func (m *pooledModule) decode(reply GuestReply, callErr error) (Result, error) {
	if callErr != nil {
		return Result{}, m.classifier.Classify(callErr)
	}
	if m.program.Source().Entry == EntryPredicate {
		return Result{Bool: reply.Bool}, nil
	}
	d, err := m.marshaller.FromGuest(reply.Output)
	var oe *boundary.OutputError
	if errors.As(err, &oe) {
		return Result{}, m.classifier.New(ErrorInvalidOutput, oe.Reason)
	}
	if err != nil {
		return Result{}, m.classifier.New(ErrorInternal, err.Error())
	}
	return Result{Outputs: d.Outputs, Dropped: d.Dropped}, nil
}

// done stamps the duration, reports metrics (pool gauges when the pool size changed) and returns.
func (m *pooledModule) done(start time.Time, res Result, err error) (Result, error) {
	res.Duration = time.Since(start)
	out := CallOutcome{StepPath: m.program.Source().StepPath, Duration: res.Duration, CPUTime: res.CPUTime, MemoryBytes: res.MemoryBytes}
	if se, ok := AsError(err); ok {
		out.Kind = se.Kind
	}
	m.observer.CallObserved(m.scope, out)
	if size := int64(m.pool.Stats().Size); m.poolSize.Swap(size) != size && m.route != nil {
		m.route.observePools()
	}
	return res, err
}
