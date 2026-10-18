package runner

import "sync"

// FetchPauser is the consumer surface that pauses and resumes partition fetching.
type FetchPauser interface {
	PauseFetchPartitions(map[string][]int32) map[string][]int32
	ResumeFetchPartitions(map[string][]int32)
}

// BackpressureGate pauses fetching of a partition while any reason holds it (queue full, producer backoff)
// instead of buffering, and resumes it when the last reason is released.
type BackpressureGate struct {
	mu      sync.Mutex
	pauser  FetchPauser
	reasons map[TopicPartition]map[PauseReason]int
}

// NewBackpressureGate builds a gate over the consumer.
func NewBackpressureGate(pauser FetchPauser) *BackpressureGate {
	return &BackpressureGate{pauser: pauser, reasons: map[TopicPartition]map[PauseReason]int{}}
}

// Pause adds a hold on tp; the first hold pauses fetching.
func (g *BackpressureGate) Pause(tp TopicPartition, reason PauseReason) {
	g.mu.Lock()
	defer g.mu.Unlock()
	held := g.reasons[tp]
	if held == nil {
		held = map[PauseReason]int{}
		g.reasons[tp] = held
	}
	if len(held) == 0 && g.pauser != nil {
		g.pauser.PauseFetchPartitions(map[string][]int32{tp.Topic: {tp.Partition}})
	}
	held[reason]++
}

// Resume releases one hold; fetching resumes when no hold remains.
func (g *BackpressureGate) Resume(tp TopicPartition, reason PauseReason) {
	g.mu.Lock()
	defer g.mu.Unlock()
	held := g.reasons[tp]
	if held[reason] == 0 {
		return
	}
	held[reason]--
	if held[reason] == 0 {
		delete(held, reason)
	}
	if len(held) == 0 && g.pauser != nil {
		g.pauser.ResumeFetchPartitions(map[string][]int32{tp.Topic: {tp.Partition}})
	}
}

// Paused reports whether tp is held.
func (g *BackpressureGate) Paused(tp TopicPartition) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.reasons[tp]) > 0
}

// Forget drops holds of revoked partitions without touching the consumer.
func (g *BackpressureGate) Forget(tps []TopicPartition) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, tp := range tps {
		delete(g.reasons, tp)
	}
}

// ReleaseAll drops every hold and resumes fetching of the held partitions (aborted transactional batch).
func (g *BackpressureGate) ReleaseAll() {
	g.mu.Lock()
	defer g.mu.Unlock()
	held := map[string][]int32{}
	for tp, reasons := range g.reasons {
		if len(reasons) > 0 {
			held[tp.Topic] = append(held[tp.Topic], tp.Partition)
		}
	}
	g.reasons = map[TopicPartition]map[PauseReason]int{}
	if len(held) > 0 && g.pauser != nil {
		g.pauser.ResumeFetchPartitions(held)
	}
}
