package runner

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
	"github.com/eventrouter/router/internal/runtime/membudget"
	"github.com/eventrouter/router/internal/runtime/producers"
	"github.com/eventrouter/router/internal/runtime/retry"
	"github.com/eventrouter/router/pkg/config"
)

// Deps are the worker-wide collaborators every runner is built from.
type Deps struct {
	Opener     ClientOpener
	Producers  *producers.Pool
	Delivery   *delivery.Manager
	Classifier failures.ErrorClassifier
	DLQTargets failures.DLQTargetResolver
	Encoder    failures.RecordEncoder
	Metrics    *Metrics
	Timings    domain.Timings
	Retry      retry.Policy
	Log        *slog.Logger
	Now        func() time.Time
	Tail       TailTap
	Budget     *membudget.Budget
	InFlight   InFlightTuning
}

// RouteRunnerFactory builds RouteRunners.
type RouteRunnerFactory struct {
	Deps Deps
}

// New implements Factory.
func (f RouteRunnerFactory) New(v CompiledVersion) Runner {
	return &RouteRunner{deps: f.Deps, version: v, state: domain.RunnerStarting, lanes: map[TopicPartition]*PartitionLane{},
		watermarks: map[TopicPartition]int64{}, halts: make(chan *failures.HaltError, 1), pollDone: make(chan struct{}), barrier: NewBatchBarrier()}
}

// RouteRunner runs one route version: consumer, lanes, record flow, commits.
type RouteRunner struct {
	deps       Deps
	mu         sync.Mutex
	version    CompiledVersion
	state      domain.RunnerState
	lastErr    *failures.HaltError
	partitions int
	consumer   *SourceConsumer
	strategy   delivery.Strategy
	cycle      PollCycle
	barrier    *BatchBarrier
	flow       *RecordFlow
	tracker    *OffsetTracker
	gate       *BackpressureGate
	committer  *CommitScheduler
	account    *membudget.Account
	tuner      *InFlightTuner
	lanes      map[TopicPartition]*PartitionLane
	watermarks map[TopicPartition]int64
	probed     ProbedLag
	halts      chan *failures.HaltError
	runCtx     context.Context
	stopPoll   context.CancelFunc
	stopFlow   context.CancelFunc
	pollDone   chan struct{}
	stopOnce   sync.Once
	retries    *RetryRuntime
	// launched is set once launch built tracker, gate and committer; rebalance callbacks before it are no-ops.
	launched atomic.Bool
}

// Version implements Runner.
func (r *RouteRunner) Version() CompiledVersion {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.version
}

// State implements Runner.
func (r *RouteRunner) State() domain.RunnerState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

// Halts implements Runner.
func (r *RouteRunner) Halts() <-chan *failures.HaltError { return r.halts }

// Load implements Runner (scheduler load weight of this replica).
func (r *RouteRunner) Load() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return domain.LoadWeights{}.Route(r.partitions)
}

// Start opens clients, ensures the DLQ topic and begins polling; a *failures.HaltError means do not retry.
func (r *RouteRunner) Start(ctx context.Context) error {
	v := r.Version()
	r.partitions = SourceTopology{Opener: r.deps.Opener}.Partitions(ctx, v)
	mode := delivery.DeliveryModes{}.Of(v.Config)
	tuner, err := NewInFlightTuner(r.deps.InFlight, v.Config.Limits.InFlight(), r.deps.Metrics.For(v))
	if err != nil {
		return r.startFailed(err)
	}
	r.tuner = tuner
	if err := r.openSource(ctx, v, mode); err != nil {
		return r.startFailed(err)
	}
	if err := r.openDelivery(ctx, v, mode); err != nil {
		return r.startFailed(err)
	}
	if err := r.ensureDLQ(ctx, v); err != nil {
		return r.startFailed(err)
	}
	if err := r.ensureDestinations(ctx, v); err != nil {
		return r.startFailed(err)
	}
	if err := r.startRetryTopics(ctx, v); err != nil {
		return r.startFailed(err)
	}
	r.launch(v)
	return nil
}

// openSource opens the consumer the delivery mode needs (a transact session for exactly_once).
func (r *RouteRunner) openSource(ctx context.Context, v CompiledVersion, mode config.DeliveryMode) error {
	source, err := NewSourceModes(r.deps.Producers.TransactionalOptions()).For(mode)
	if err != nil {
		return ModeRefusal{}.Halt(err)
	}
	consumer, err := OpenSourceConsumer(ctx, r.deps.Opener, v, r, source, SourceFetch{Budget: r.deps.Budget})
	if err != nil {
		return ModeRefusal{}.Halt(err)
	}
	r.consumer = consumer
	return nil
}

// openDelivery builds the delivery strategy; a mode this route may not use halts it with the reason.
func (r *RouteRunner) openDelivery(ctx context.Context, v CompiledVersion, mode config.DeliveryMode) error {
	strategy, err := r.deps.Delivery.For(ctx, mode, delivery.Deps{Consumer: r.consumer.Client(), Session: r.consumer.Session(),
		Producers: r.deps.Producers, Connections: v.Connections, Source: v.Config.Source.ConnectionID, Owner: v.Ref.RouteID, Abort: r.barrier.Fail})
	if err != nil {
		return ModeRefusal{}.Halt(err)
	}
	r.strategy = strategy
	return nil
}

// ModeRefusal turns delivery-mode refusals (unavailable mode, cross-connection exactly_once) into halts.
type ModeRefusal struct{}

// Halt wraps refusals as delivery_mode_forbidden halts and returns other errors unchanged.
func (ModeRefusal) Halt(err error) error {
	if errors.Is(err, delivery.ErrModeUnavailable) || errors.Is(err, delivery.ErrCrossConnection) || errors.Is(err, delivery.ErrNoSession) {
		return &failures.HaltError{Reason: domain.HaltDeliveryModeForbidden, Category: domain.CategorySourceFatal, Message: err.Error()}
	}
	return err
}

// launch builds the per-runner pipeline objects and starts the poll, commit and in-flight tuning loops.
func (r *RouteRunner) launch(v CompiledVersion) {
	tuner := r.tuner
	runCtx, stopPoll := context.WithCancel(context.Background())
	flowCtx, stopFlow := context.WithCancel(context.Background())
	r.runCtx, r.stopPoll, r.stopFlow = runCtx, stopPoll, stopFlow
	r.tracker = NewOffsetTracker()
	r.gate = NewBackpressureGate(r.consumer.Client())
	r.account = r.openAccount(v)
	r.committer = NewCommitScheduler(r.tracker, r.strategy, r.deps.Timings.Commit, r.deps.Log)
	r.flow = &RecordFlow{delivery: r.strategy, classifier: r.deps.Classifier, tracker: r.tracker, gate: r.gate,
		limiter: tuner.Limiter(), retry: r.deps.Retry, halt: r.signalHalt, barrier: r.barrier, tail: NewTailHook(r.deps.Tail)}
	r.flow.useBase(flowCtx, stopFlow)
	r.flow.state.Store(r.versionState(v))
	r.cycle = NewPollCycles().For(r.strategy.Batching())
	r.launched.Store(true)
	r.setState(domain.RunnerRunning)
	go r.poll(runCtx)
	go r.committer.Run(runCtx)
	go tuner.Run(runCtx)
	r.launchRetries(runCtx)
	if r.deps.Timings.LagProbe > 0 {
		go LagProbeLoop{Probe: NewLagProbe(r.consumer.Client(), ConsumerGroups{}.Of(v)), Interval: r.deps.Timings.LagProbe, Apply: r.applyProbe}.Run(runCtx)
	}
}

// applyProbe merges the broker view into the watermarks and re-emits the lag gauges.
func (r *RouteRunner) applyProbe(p ProbedLag) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.probed = p
	for tp, part := range p {
		r.watermarks[tp] = max(r.watermarks[tp], part.End)
	}
	r.lagLocked()
}

// versionState builds the swappable part of the flow for v.
func (r *RouteRunner) versionState(v CompiledVersion) *versionState {
	return &versionState{version: v, processor: NewRecordProcessor(v), policies: failures.NewDefaultPolicyManager(r.dlqWriter(v, r.strategy)),
		metrics: r.deps.Metrics.For(v), retries: r.retryRoute(v)}
}

// ensureDLQ creates the DLQ topic when the route's policy writes dead letters.
func (r *RouteRunner) ensureDLQ(ctx context.Context, v CompiledVersion) error {
	if !failures.NewDefaultPolicyManager(nil).UsesDLQ(v.Config.ErrorPolicy) {
		return nil
	}
	target := r.deps.DLQTargets.Resolve(v.Config)
	admin, err := r.deps.Opener.Admin(ctx, v.Connections[target.ConnectionID])
	if err != nil {
		return err
	}
	defer admin.Close()
	return failures.TopicEnsurer{}.Ensure(ctx, admin.Admin(), target.Topic, r.partitions)
}

// ensureDestinations creates the missing destination topics of branches with destination.createTopic.
func (r *RouteRunner) ensureDestinations(ctx context.Context, v CompiledVersion) error {
	for _, b := range v.Config.Branches {
		if b.Destination.CreateTopic == nil {
			continue
		}
		if err := r.ensureDestination(ctx, v, b.Destination); err != nil {
			return err
		}
	}
	return nil
}

func (r *RouteRunner) ensureDestination(ctx context.Context, v CompiledVersion, dst config.Destination) error {
	admin, err := r.deps.Opener.Admin(ctx, v.Connections[dst.ConnectionID])
	if err != nil {
		return err
	}
	defer admin.Close()
	return failures.TopicEnsurer{}.EnsureSpec(ctx, admin.Admin(), failures.DestinationTopicSpec{}.Of(dst), failures.TopicPurposes.Destination)
}

// startFailed releases what Start opened and records the error.
func (r *RouteRunner) startFailed(err error) error {
	if r.strategy != nil {
		_ = r.strategy.Close(context.Background())
	}
	if r.consumer != nil {
		r.consumer.Close()
	}
	r.closeRetries(context.Background())
	r.recordError(err)
	close(r.pollDone)
	return err
}

// poll hands fetched records to lanes until the poll context ends.
func (r *RouteRunner) poll(ctx context.Context) {
	defer close(r.pollDone)
	for {
		fetches := r.consumer.Poll(ctx)
		if ctx.Err() != nil || fetches.IsClientClosed() {
			return
		}
		fetches.EachError(func(_ string, _ int32, err error) { r.sourceError(err) })
		r.cycle.Run(ctx, r, fetches)
		r.consumer.AllowRebalance()
	}
}

// sourceError halts on fatal source errors; transient ones are retried by the client.
func (r *RouteRunner) sourceError(err error) {
	cat := r.deps.Classifier.ClassifySource(err)
	if cat == domain.CategorySourceFatal {
		r.signalHalt(&failures.HaltError{Reason: domain.HaltSourceFatal, Category: cat, Message: err.Error()})
		return
	}
	r.deps.Log.Debug("runtime: transient source error", string(logging.AttrRouteID), r.version.Ref.RouteID, string(logging.AttrError), err)
}

// dispatch records the partition high watermark and enqueues its records on the lane.
func (r *RouteRunner) dispatch(ctx context.Context, p kgo.FetchTopicPartition) {
	tp := TopicPartition{Topic: p.Topic, Partition: p.Partition}
	r.mu.Lock()
	r.watermarks[tp] = p.HighWatermark
	r.mu.Unlock()
	if len(p.Records) == 0 {
		return
	}
	r.laneFor(ctx, tp).Enqueue(p.Records)
}

// laneFor returns the lane of tp, starting it on first use.
func (r *RouteRunner) laneFor(ctx context.Context, tp TopicPartition) *PartitionLane {
	r.mu.Lock()
	defer r.mu.Unlock()
	if l, ok := r.lanes[tp]; ok {
		return l
	}
	l := NewPartitionLane(tp, r.gate).WithAccount(r.account)
	r.lanes[tp] = l
	go l.Run(ctx, r.flow)
	return l
}

// signalHalt records the halt and notifies the manager once.
func (r *RouteRunner) signalHalt(h *failures.HaltError) {
	r.recordError(h)
	select {
	case r.halts <- h:
	default:
	}
}

func (r *RouteRunner) recordError(err error) {
	h := &failures.HaltError{Category: domain.CategorySourceTransient, Message: err.Error()}
	errors.As(err, &h)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastErr = h
	r.setStateLocked(domain.RunnerError)
}

func (r *RouteRunner) setState(s domain.RunnerState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setStateLocked(s)
}

func (r *RouteRunner) setStateLocked(s domain.RunnerState) {
	if r.state == domain.RunnerError && s != domain.RunnerStopped {
		return
	}
	r.deps.Metrics.For(r.version).State(r.state, s)
	r.state = s
}
