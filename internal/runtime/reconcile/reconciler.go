package reconcile

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/retry"
	"github.com/eventrouter/router/internal/runtime/runner"
)

// ControlPlane is the worker's view of the API's internal endpoints.
type ControlPlane interface {
	Heartbeat(ctx context.Context, hb domain.Heartbeat) (domain.HeartbeatReply, error)
	Assignments(ctx context.Context, etag string) (domain.AssignmentSet, bool, error)
	VersionConfig(ctx context.Context, versionID string) ([]byte, error)
	ReportStatus(ctx context.Context, replicas []domain.ReplicaStatus) error
	ReportHalt(ctx context.Context, h domain.HaltReport) error
	Changes(ctx context.Context) (<-chan domain.ChangeHint, error)
}

// ConfigCache is the on-disk cache of assignments and version documents.
type ConfigCache interface {
	SaveAssignments(set domain.AssignmentSet) error
	LoadAssignments() (domain.AssignmentSet, error)
	SaveVersion(ref domain.VersionRef, doc []byte) error
	LoadVersion(ref domain.VersionRef) ([]byte, error)
}

// Runners is the runner manager surface reconciliation drives.
type Runners interface {
	Start(ctx context.Context, v runner.CompiledVersion) error
	Swap(ctx context.Context, next runner.CompiledVersion) error
	Restart(ctx context.Context, next runner.CompiledVersion) error
	Remove(ctx context.Context, routeID string, reason domain.StopReason)
	RecordFailure(ref domain.VersionRef, category domain.ErrorCategory, message string)
	RecordUpgradeFailure(ref domain.VersionRef, category domain.ErrorCategory, message string)
	Current(routeID string) (runner.Current, bool)
	RouteIDs() []string
	Snapshot() []domain.ReplicaStatus
	CapacityUsed() float64
	StopAll(ctx context.Context)
}

// Deps are the reconciler's collaborators.
type Deps struct {
	ControlPlane ControlPlane
	Cache        ConfigCache
	Runners      Runners
	Compiler     VersionCompiler
	Reload       ReloadClassifier
	Halts        *HaltQueue
	Identity     domain.WorkerIdentity
	Timings      domain.Timings
	Retry        retry.Policy
	Metrics      *Metrics
	Log          *slog.Logger
}

// Reconciler converges this worker's runners on the control plane's explicit assignments (design 5.4, 5.7).
type Reconciler struct {
	d        Deps
	mu       sync.Mutex
	state    sync.Mutex
	etag     string
	last     domain.AssignmentSet
	refused  map[string]bool
	degraded atomic.Bool
	draining atomic.Bool
	trigger  chan struct{}
}

// New builds the reconciler.
func New(d Deps) *Reconciler {
	return &Reconciler{d: d, refused: map[string]bool{}, trigger: make(chan struct{}, 1)}
}

// Degraded reports whether the control plane was unreachable on the last call.
func (r *Reconciler) Degraded() bool { return r.degraded.Load() }

// Trigger asks for a reconciliation soon (change hints, halts).
func (r *Reconciler) Trigger() {
	select {
	case r.trigger <- struct{}{}:
	default:
	}
}

// Run loops until ctx ends, then drains every runner (graceful shutdown, design 5.10). Heartbeats run on their
// own goroutine: a long reconciliation (hundreds of routes starting) must not starve lease renewal.
func (r *Reconciler) Run(ctx context.Context) {
	go r.watchChanges(ctx)
	r.heartbeat(ctx)
	beats := r.heartbeats(ctx)
	r.Reconcile(ctx)
	resync := time.NewTicker(r.d.Timings.Resync)
	defer resync.Stop()
	for {
		select {
		case <-ctx.Done():
			<-beats
			r.shutdown()
			return
		case <-resync.C:
			r.Reconcile(ctx)
		case <-r.trigger:
			r.Reconcile(ctx)
		}
	}
}

// heartbeats renews liveness and leases every heartbeat interval until ctx ends; the channel closes on exit.
func (r *Reconciler) heartbeats(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		beat := time.NewTicker(r.d.Timings.Heartbeat)
		defer beat.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-beat.C:
				r.heartbeat(ctx)
			}
		}
	}()
	return done
}

// Reconcile runs one pass: desired set, start/upgrade/stop, report.
func (r *Reconciler) Reconcile(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	set, explicit := r.desired(ctx)
	for _, a := range set.Assignments {
		r.ensure(ctx, a)
	}
	if explicit {
		r.removeUnassigned(ctx, set)
	}
	r.d.Halts.Flush(ctx, r.d.ControlPlane)
	r.report(ctx)
}

// desired returns the explicit answer, or the last known set while the control plane is down.
func (r *Reconciler) desired(ctx context.Context) (domain.AssignmentSet, bool) {
	set, notModified, err := r.d.ControlPlane.Assignments(ctx, r.currentETag())
	if err != nil {
		r.setDegraded(true)
		r.d.Log.Warn("runtime: control plane unreachable; keeping current runners", string(logging.AttrError), err)
		return r.cached(), false
	}
	r.setDegraded(false)
	r.state.Lock()
	defer r.state.Unlock()
	if notModified {
		return r.last, true
	}
	r.etag, r.last = set.ETag, set
	if err := r.d.Cache.SaveAssignments(set); err != nil {
		r.d.Log.Warn("runtime: assignment cache write failed", string(logging.AttrError), err)
	}
	return set, true
}

// currentETag returns the ETag of the last explicit answer.
func (r *Reconciler) currentETag() string {
	r.state.Lock()
	defer r.state.Unlock()
	return r.etag
}

// cached returns the in-memory set, else the on-disk one (worker restarted while the API is down).
func (r *Reconciler) cached() domain.AssignmentSet {
	r.state.Lock()
	defer r.state.Unlock()
	if r.last.Assignments != nil {
		return r.last
	}
	set, err := r.d.Cache.LoadAssignments()
	if err != nil {
		return domain.AssignmentSet{}
	}
	r.last = set
	return set
}

// removeUnassigned stops runners whose lease is absent from the explicit answer (never on silence).
func (r *Reconciler) removeUnassigned(ctx context.Context, set domain.AssignmentSet) {
	wanted := set.ByRoute()
	for _, id := range r.d.Runners.RouteIDs() {
		if _, ok := wanted[id]; !ok {
			r.d.Runners.Remove(ctx, id, domain.StopUnassigned)
		}
	}
	r.state.Lock()
	defer r.state.Unlock()
	for id := range r.refused {
		if _, ok := wanted[id]; !ok {
			delete(r.refused, id)
		}
	}
}

// report sends every replica status (failures mark the control plane down).
func (r *Reconciler) report(ctx context.Context) {
	if err := r.d.ControlPlane.ReportStatus(ctx, r.d.Runners.Snapshot()); err != nil {
		r.setDegraded(true)
	}
}

// heartbeat reports liveness and capacity; a changed assignments ETag triggers reconciliation.
func (r *Reconciler) heartbeat(ctx context.Context) {
	used := r.d.Runners.CapacityUsed()
	r.d.Metrics.Capacity(used)
	reply, err := r.d.ControlPlane.Heartbeat(ctx, r.heartbeatBody(used))
	if err != nil {
		r.setDegraded(true)
		return
	}
	r.setDegraded(false)
	if reply.AssignmentsETag != r.currentETag() {
		r.Trigger()
	}
}

func (r *Reconciler) heartbeatBody(used float64) domain.Heartbeat {
	r.state.Lock()
	defer r.state.Unlock()
	refused := make([]string, 0, len(r.refused))
	for id := range r.refused {
		refused = append(refused, id)
	}
	id := r.d.Identity
	return domain.Heartbeat{Host: id.Host, Zone: id.Zone, Version: id.Version, Capacity: id.Capacity, CapacityUsed: used,
		RefusedRouteIDs: refused, Draining: r.draining.Load()}
}

// watchChanges relays change hints into reconciliations, reconnecting with backoff.
func (r *Reconciler) watchChanges(ctx context.Context) {
	for attempt := 0; ctx.Err() == nil; attempt++ {
		hints, err := r.d.ControlPlane.Changes(ctx)
		if err == nil {
			attempt = 0
			for range hints {
				r.Trigger()
			}
		}
		if r.d.Retry.Wait(ctx, attempt) != nil {
			return
		}
	}
}

// shutdown announces draining, then drains every runner within the grace period.
func (r *Reconciler) shutdown() {
	r.draining.Store(true)
	ctx, cancel := context.WithTimeout(context.Background(), r.d.Timings.StopGrace)
	defer cancel()
	r.heartbeat(ctx)
	r.d.Runners.StopAll(ctx)
	r.report(ctx)
}

func (r *Reconciler) setDegraded(down bool) {
	r.degraded.Store(down)
	r.d.Metrics.ControlPlaneUp(!down)
}
