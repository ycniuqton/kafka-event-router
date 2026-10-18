package kafka

import (
	"sync"
	"time"
)

// SamplingBudget bounds one sampling load by events, bytes and a deadline; it is safe for concurrent use.
// The first limit that refuses a record becomes the stop reason.
type SamplingBudget struct {
	mu        sync.Mutex
	maxEvents int
	maxBytes  int64
	deadline  time.Time
	events    int
	bytes     int64
	reason    StopReason
}

// NewSamplingBudget builds a budget; a zero deadline means no time limit.
func NewSamplingBudget(maxEvents int, maxBytes int64, deadline time.Time) *SamplingBudget {
	return &SamplingBudget{maxEvents: maxEvents, maxBytes: maxBytes, deadline: deadline}
}

// Admit counts a record of size bytes at time now, or refuses it and records why.
func (b *SamplingBudget) Admit(size int64, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.reason != StopNone {
		return false
	}
	b.reason = b.refusal(size, now)
	if b.reason != StopNone {
		return false
	}
	b.events++
	b.bytes += size
	return true
}

// Expired reports whether the deadline passed (and records it as the stop reason).
func (b *SamplingBudget) Expired(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.reason == StopNone && !b.deadline.IsZero() && !now.Before(b.deadline) {
		b.reason = StopDeadline
	}
	return b.reason == StopDeadline
}

// Remaining returns the time left until the deadline; bounded is false without a deadline.
func (b *SamplingBudget) Remaining(now time.Time) (left time.Duration, bounded bool) {
	if b.deadline.IsZero() {
		return 0, false
	}
	return b.deadline.Sub(now), true
}

// Exhausted reports whether any limit refused a record.
func (b *SamplingBudget) Exhausted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reason != StopNone
}

// Reason returns the stop reason, or StopComplete when no limit was hit.
func (b *SamplingBudget) Reason() StopReason {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.reason == StopNone {
		return StopComplete
	}
	return b.reason
}

// Used returns the admitted events and bytes.
func (b *SamplingBudget) Used() (int, int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.events, b.bytes
}

func (b *SamplingBudget) refusal(size int64, now time.Time) StopReason {
	switch {
	case !b.deadline.IsZero() && !now.Before(b.deadline):
		return StopDeadline
	case b.events >= b.maxEvents:
		return StopMaxEvents
	case b.bytes+size > b.maxBytes:
		return StopMaxBytes
	}
	return StopNone
}
