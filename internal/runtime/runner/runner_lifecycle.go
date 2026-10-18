package runner

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/pkg/kafka"
)

// Revoked implements RebalanceListener: drain the revoked lanes, commit their marks, forget them.
func (r *RouteRunner) Revoked(ctx context.Context, tps []TopicPartition) {
	lanes := r.takeLanes(tps)
	r.drain(lanes)
	if err := r.committer.CommitNow(ctx, tps); err != nil {
		r.deps.Log.Warn("runtime: commit on revoke failed", string(logging.AttrRouteID), r.Version().Ref.RouteID, string(logging.AttrError), err)
	}
	r.forget(tps)
}

// Lost implements RebalanceListener: partitions are gone, nothing can be committed.
func (r *RouteRunner) Lost(tps []TopicPartition) {
	r.barrier.Fail()
	for _, l := range r.takeLanes(tps) {
		l.Stop()
	}
	r.forget(tps)
}

// Swap replaces pipeline and policies between records without leaving the consumer group.
func (r *RouteRunner) Swap(ctx context.Context, next CompiledVersion) error {
	r.flow.swapLock.Lock()
	defer r.flow.swapLock.Unlock()
	r.waitIdle(r.allLanes())
	if err := r.committer.CommitNow(ctx, nil); err != nil {
		r.deps.Log.Warn("runtime: commit before swap failed", string(logging.AttrRouteID), next.Ref.RouteID, string(logging.AttrError), err)
	}
	prev := r.flow.state.Load()
	r.flow.state.Store(r.versionState(next))
	r.mu.Lock()
	r.retagMetricsLocked(next)
	r.mu.Unlock()
	return prev.processor.Close(ctx)
}

// retagMetricsLocked moves the runner's metric series from the replaced version to next,
// so a hot reload leaves no stale series of the old version behind.
func (r *RouteRunner) retagMetricsLocked(next CompiledVersion) {
	r.deps.Metrics.For(r.version).State(r.state, domain.RunnerStopped)
	r.version = next
	r.deps.Metrics.For(next).State(domain.RunnerStopped, r.state)
}

// Stop stops fetching, drains in-flight records, commits and leaves the group.
func (r *RouteRunner) Stop(ctx context.Context, reason domain.StopReason) error {
	r.stopOnce.Do(func() {
		if r.consumer == nil || r.flow == nil {
			return
		}
		r.setState(domain.RunnerDraining)
		r.stopPoll()
		<-r.pollDone
		r.drain(r.takeLanes(nil))
		if err := r.committer.CommitNow(ctx, nil); err != nil {
			r.deps.Log.Warn("runtime: final commit failed", string(logging.AttrRouteID), r.Version().Ref.RouteID, string(logging.AttrError), err)
		}
		r.stopFlow()
		r.closeDelivery(ctx)
		r.consumer.Close()
		_ = r.flow.state.Load().processor.Close(ctx)
		r.finalState(reason)
	})
	return nil
}

// closeDelivery releases the producers, flushing at most the drain timeout: with the destination down a
// flush never ends, and unflushed records are uncommitted, so they are read again (at-least-once).
func (r *RouteRunner) closeDelivery(ctx context.Context) {
	closeCtx, cancel := context.WithTimeout(ctx, r.deps.Timings.Drain)
	defer cancel()
	_ = r.strategy.Close(closeCtx)
}

// finalState is error after a halt, stopped otherwise.
func (r *RouteRunner) finalState(reason domain.StopReason) {
	if reason == domain.StopHalt {
		r.setState(domain.RunnerError)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastErr = nil
	r.setStateLocked(domain.RunnerStopped)
}

// Status implements Runner.
func (r *RouteRunner) Status() domain.ReplicaStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := domain.ReplicaStatus{RouteID: r.version.Ref.RouteID, VersionID: r.version.Ref.VersionID, VersionNo: r.version.Ref.VersionNo,
		State: r.state, Partitions: r.partitions, Lag: r.lagLocked()}
	if r.lastErr != nil {
		s.ErrorCategory, s.ErrorMessage = r.lastErr.Category, r.lastErr.Message
	}
	if r.flow != nil {
		s.CPUMs = r.flow.cpu.Load()
	}
	for _, l := range r.lanes {
		s.MemBytes += l.QueuedBytes()
	}
	return s
}

// lagLocked sums the lag of every known partition and emits it per partition.
func (r *RouteRunner) lagLocked() int64 {
	if r.committer == nil {
		return 0
	}
	var total int64
	metrics := r.deps.Metrics.For(r.version)
	for tp := range r.watermarks {
		lag, ok := r.partitionLagLocked(tp)
		if !ok {
			continue
		}
		metrics.Lag(tp.Partition, lag)
		total += lag
	}
	return total
}

// partitionLagLocked is high watermark − local commit; before the first local commit it counts from the first
// unfinished record, else from the group's commit; unknown (idle, never committed) reports nothing.
func (r *RouteRunner) partitionLagLocked(tp TopicPartition) (int64, bool) {
	if committed, ok := r.committer.Committed(tp); ok {
		return max(r.watermarks[tp]-committed, 0), true
	}
	if pos, ok := r.tracker.Position(tp); ok {
		return max(r.watermarks[tp]-pos, 0), true
	}
	if p, ok := r.probed[tp]; ok && p.Lag != int64(NoLag) {
		return p.Lag, true
	}
	return 0, false
}

// takeLanes removes and returns the lanes of tps (nil = all).
func (r *RouteRunner) takeLanes(tps []TopicPartition) []*PartitionLane {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*PartitionLane{}
	if tps == nil {
		for tp, l := range r.lanes {
			out = append(out, l)
			delete(r.lanes, tp)
		}
		return out
	}
	for _, tp := range tps {
		if l, ok := r.lanes[tp]; ok {
			out = append(out, l)
			delete(r.lanes, tp)
		}
	}
	return out
}

func (r *RouteRunner) allLanes() []*PartitionLane {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*PartitionLane, 0, len(r.lanes))
	for _, l := range r.lanes {
		out = append(out, l)
	}
	return out
}

// drain stops the lanes and waits for their in-flight records (bounded by the drain timeout).
func (r *RouteRunner) drain(lanes []*PartitionLane) {
	for _, l := range lanes {
		l.Stop()
	}
	r.waitIdle(lanes)
}

// waitIdle waits until no lane has a dispatched record in flight or the drain timeout passes.
func (r *RouteRunner) waitIdle(lanes []*PartitionLane) {
	deadline := time.Now().Add(r.deps.Timings.Drain)
	for time.Now().Before(deadline) && Lanes(lanes).InFlight() > 0 {
		time.Sleep(domain.DrainPoll.D())
	}
}

func (r *RouteRunner) forget(tps []TopicPartition) {
	r.tracker.Forget(tps)
	r.gate.Forget(tps)
	r.committer.Forget(tps)
	r.mu.Lock()
	defer r.mu.Unlock()
	metrics := r.deps.Metrics.For(r.version)
	for _, tp := range tps {
		delete(r.watermarks, tp)
		delete(r.probed, tp)
		metrics.Lag(tp.Partition, 0)
	}
}

// rewindBatch abandons the unfinished records of a failed transactional batch: their delivery context ends,
// lanes drop queued records and wait for in-flight ones, and fetch pauses are lifted for the re-read.
func (r *RouteRunner) rewindBatch(scope *batchScope) {
	scope.cancel()
	r.drain(r.takeLanes(nil))
	r.gate.ReleaseAll()
}

// forgetBatch drops offset state after an abort: the session rewound the consumer to the last commit.
func (r *RouteRunner) forgetBatch() {
	tps := r.tracker.Tracked()
	r.tracker.Forget(tps)
	r.committer.Forget(tps)
}

// Lanes is a group of partition lanes.
type Lanes []*PartitionLane

// InFlight sums dispatched, unfinished records.
func (ls Lanes) InFlight() int64 {
	var n int64
	for _, l := range ls {
		n += l.InFlight()
	}
	return n
}

// SourceTopology reads the partition count of a route's source topics (0 when unknown, e.g. patterns).
type SourceTopology struct {
	Opener ClientOpener
}

// Partitions describes the source topics through an admin client.
func (s SourceTopology) Partitions(ctx context.Context, v CompiledVersion) int {
	if len(v.Config.Source.Topics) == 0 {
		return 0
	}
	admin, err := s.Opener.Admin(ctx, v.Connections[v.Config.Source.ConnectionID])
	if err != nil {
		return 0
	}
	defer admin.Close()
	infos, err := admin.Admin().DescribeTopics(ctx, v.Config.Source.Topics...)
	if err != nil {
		return 0
	}
	return TopicInfos(infos).Partitions()
}

// TopicInfos sums partitions of described topics.
type TopicInfos []kafka.TopicInfo

// Partitions returns the total partition count.
func (ts TopicInfos) Partitions() int {
	n := 0
	for _, t := range ts {
		n += t.Partitions
	}
	return n
}
