package pacing

import "time"

// Limits bounds one pacing decision: the most records released per call and the idle sleep between calls.
var Limits = struct {
	MaxBurst int64
	Idle     time.Duration
}{MaxBurst: 512, Idle: time.Millisecond}

// Pacer releases records open-loop at a constant rate measured from the start of a step; after a stall it catches up
// in bursts of at most MaxBurst per call, so latency is not hidden by coordinated omission.
type Pacer struct {
	rate  float64
	start time.Time
	sent  int64
}

// NewPacer starts pacing at rate records/s from now; rate 0 means unlimited.
func NewPacer(rate float64, now time.Time) *Pacer {
	return &Pacer{rate: rate, start: now}
}

// Due returns how many records may be sent at now, and records them as sent.
func (p *Pacer) Due(now time.Time) int64 {
	if p.rate <= 0 {
		p.sent += Limits.MaxBurst
		return Limits.MaxBurst
	}
	allowed := int64(now.Sub(p.start).Seconds() * p.rate)
	due := min(allowed-p.sent, Limits.MaxBurst)
	if due <= 0 {
		return 0
	}
	p.sent += due
	return due
}

// Sent is the number of records released so far.
func (p *Pacer) Sent() int64 {
	return p.sent
}
