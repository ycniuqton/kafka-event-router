package runner

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
)

// HaltSink receives halts so the control plane can be told (retried while it is unreachable).
type HaltSink interface {
	Halted(routeID string, ref domain.VersionRef, h *failures.HaltError)
}

// Current describes what runs for a route on this worker.
type Current struct {
	Version CompiledVersion
	Ref     domain.VersionRef
	State   domain.RunnerState
	Failed  bool
}

// entry is one route on this worker: a live runner, or a failed placeholder that must not be retried
// until its version changes (compile failure, halt).
type entry struct {
	routeID  string
	runner   Runner
	failed   *domain.ReplicaStatus
	override *domain.ReplicaStatus
	stop     chan struct{}
}

// failedReport returns the failure placeholder or upgrade failure, if any.
func (e *entry) failedReport() *domain.ReplicaStatus {
	if e.failed != nil {
		return e.failed
	}
	return e.override
}

// report returns what this entry tells the control plane.
func (e *entry) report() domain.ReplicaStatus {
	if e.failed != nil {
		return *e.failed
	}
	if e.override != nil {
		return *e.override
	}
	return e.runner.Status()
}

// Manager is the only entry point of reconciliation into runners; one route's failure never touches another.
type Manager struct {
	mu      sync.Mutex
	entries map[string]*entry
	factory Factory
	halts   HaltSink
	log     *slog.Logger
	wg      sync.WaitGroup
}

// NewManager builds the manager.
func NewManager(factory Factory, halts HaltSink, log *slog.Logger) *Manager {
	return &Manager{entries: map[string]*entry{}, factory: factory, halts: halts, log: log}
}

// Start creates and starts the runner of v; a start halt leaves a failed entry, other errors leave none.
func (m *Manager) Start(ctx context.Context, v CompiledVersion) error {
	r := m.factory.New(v)
	err := r.Start(ctx)
	var h *failures.HaltError
	if errors.As(err, &h) {
		m.setFailed(v.Ref, h.Category, h.Message)
		m.halts.Halted(v.Ref.RouteID, v.Ref, h)
		return err
	}
	if err != nil {
		return err
	}
	e := &entry{routeID: v.Ref.RouteID, runner: r, stop: make(chan struct{})}
	m.mu.Lock()
	m.entries[v.Ref.RouteID] = e
	m.mu.Unlock()
	m.wg.Add(1)
	go m.watch(r, e)
	return nil
}

// watch stops a runner that raised a halt and reports it; recoverable halts restart it instead.
func (m *Manager) watch(r Runner, e *entry) {
	defer m.wg.Done()
	var h *failures.HaltError
	select {
	case <-e.stop:
		return
	case h = <-r.Halts():
	}
	if h.Reason.Recoverable() {
		m.recover(r, e, h)
		return
	}
	ref := r.Version().Ref
	m.log.Warn("runtime: route halted", string(logging.AttrRouteID), ref.RouteID, string(logging.AttrError), h.Message)
	_ = r.Stop(context.Background(), domain.StopHalt)
	m.mu.Lock()
	if e, ok := m.entries[ref.RouteID]; ok && e.runner == r {
		s := r.Status()
		e.failed = &s
	}
	m.mu.Unlock()
	m.halts.Halted(ref.RouteID, ref, h)
}

// recover restarts a runner after a runner-local failure (e.g. a failed exactly_once transaction),
// retrying until it starts, halts for good, or the route is removed or replaced meanwhile.
func (m *Manager) recover(r Runner, e *entry, h *failures.HaltError) {
	v := r.Version()
	m.log.Warn("runtime: runner restarting after a recoverable failure", string(logging.AttrRouteID), v.Ref.RouteID, string(logging.AttrError), h.Message)
	_ = r.Stop(context.Background(), domain.StopRestart)
	for m.awaitRecovery(e) {
		err := m.Start(context.Background(), v)
		var halt *failures.HaltError
		if err == nil || errors.As(err, &halt) {
			return
		}
		m.log.Warn("runtime: runner restart failed", string(logging.AttrRouteID), v.Ref.RouteID, string(logging.AttrError), err)
	}
}

// awaitRecovery waits the recovery delay and reports whether e is still the route's entry.
func (m *Manager) awaitRecovery(e *entry) bool {
	timer := time.NewTimer(domain.RunnerRecovery.D())
	defer timer.Stop()
	select {
	case <-e.stop:
		return false
	case <-timer.C:
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entries[e.routeID] == e
}

// Swap hot-reloads a running route to next.
func (m *Manager) Swap(ctx context.Context, next CompiledVersion) error {
	r, ok := m.live(next.Ref.RouteID)
	if !ok {
		return m.Start(ctx, next)
	}
	if err := r.Swap(ctx, next); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[next.Ref.RouteID]; ok {
		e.override = nil
	}
	return nil
}

// Restart stops the current runner and starts next (source, destination or delivery changed).
func (m *Manager) Restart(ctx context.Context, next CompiledVersion) error {
	m.Remove(ctx, next.Ref.RouteID, domain.StopRestart)
	return m.Start(ctx, next)
}

// Remove stops and forgets the route's runner.
func (m *Manager) Remove(ctx context.Context, routeID string, reason domain.StopReason) {
	m.mu.Lock()
	e, ok := m.entries[routeID]
	delete(m.entries, routeID)
	m.mu.Unlock()
	if ok && e.runner != nil {
		close(e.stop)
		_ = e.runner.Stop(ctx, reason)
	}
}

// RecordFailure stores a failed placeholder (compile failure) reported until the version changes.
func (m *Manager) RecordFailure(ref domain.VersionRef, category domain.ErrorCategory, message string) {
	m.setFailed(ref, category, message)
}

// RecordUpgradeFailure reports the target version as failed while the current runner keeps running.
func (m *Manager) RecordUpgradeFailure(ref domain.VersionRef, category domain.ErrorCategory, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[ref.RouteID]
	if !ok || e.runner == nil {
		return
	}
	e.override = &domain.ReplicaStatus{RouteID: ref.RouteID, VersionID: ref.VersionID, VersionNo: ref.VersionNo, State: domain.RunnerError,
		ErrorCategory: category, ErrorMessage: message}
}

func (m *Manager) setFailed(ref domain.VersionRef, category domain.ErrorCategory, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := domain.ReplicaStatus{RouteID: ref.RouteID, VersionID: ref.VersionID, VersionNo: ref.VersionNo, State: domain.RunnerError,
		ErrorCategory: category, ErrorMessage: message}
	m.entries[ref.RouteID] = &entry{failed: &s}
}

// Current returns what runs for a route.
func (m *Manager) Current(routeID string) (Current, bool) {
	m.mu.Lock()
	e, ok := m.entries[routeID]
	m.mu.Unlock()
	if !ok {
		return Current{}, false
	}
	if bad := e.failedReport(); bad != nil {
		return Current{Ref: domain.VersionRef{RouteID: routeID, VersionID: bad.VersionID, VersionNo: bad.VersionNo}, State: domain.RunnerError, Failed: true}, true
	}
	v := e.runner.Version()
	return Current{Version: v, Ref: v.Ref, State: e.runner.State()}, true
}

// RouteIDs lists every route with an entry.
func (m *Manager) RouteIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.entries))
	for id := range m.entries {
		out = append(out, id)
	}
	return out
}

// Snapshot returns the status of every route on this worker.
func (m *Manager) Snapshot() []domain.ReplicaStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.ReplicaStatus, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, e.report())
	}
	return out
}

// CapacityUsed sums the load weights of live runners.
func (m *Manager) CapacityUsed() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0.0
	for _, e := range m.entries {
		if e.failed == nil {
			total += e.runner.Load()
		}
	}
	return total
}

// StopAll drains every runner in parallel (graceful shutdown).
func (m *Manager) StopAll(ctx context.Context) {
	var wg sync.WaitGroup
	for _, id := range m.RouteIDs() {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			m.Remove(ctx, id, domain.StopShutdown)
		}(id)
	}
	wg.Wait()
}

func (m *Manager) live(routeID string) (Runner, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[routeID]
	if !ok || e.failed != nil {
		return nil, false
	}
	return e.runner, true
}
