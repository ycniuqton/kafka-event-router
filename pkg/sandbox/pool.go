package sandbox

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eventrouter/router/pkg/sandbox/host"
)

// PoolOptions size one pool: Max is the starting number of concurrent instances (route parallelism), Scale
// the autoscaling bounds (zero = fixed at Max) and Ceiling the worker JS memory budget each slot reserves
// its per-instance memory cap from (nil = no accounting).
type PoolOptions struct {
	Max     int
	Warm    int
	MaxIdle int
	Scale   PoolBounds
	Ceiling *MemoryCeiling
}

// PoolStats is a point-in-time view of one pool; ColdStarts counts instances booted while a call
// waited (none after a pre-warm sized to the route's parallelism); MemoryBytes covers every live instance (last
// sampled size) and CPUTime is the cumulative guest execution time for capacity scoring. Capacity is the
// current number of concurrent instances the pool allows (autoscaled pools change it).
type PoolStats struct {
	StepPath    string
	Size        int
	Idle        int
	InUse       int
	Capacity    int
	Discarded   uint64
	ColdStarts  uint64
	MemoryBytes int64
	Calls       uint64
	CPUTime     time.Duration
}

// Pool hands out instances of one Program.
type Pool interface {
	Acquire(ctx context.Context) (Instance, error)
	Release(i Instance)
	Discard(ctx context.Context, i Instance)
	Charge(cpu time.Duration)
	Warm(ctx context.Context, target int) (int, error)
	Stats() PoolStats
	Close(ctx context.Context) error
}

// ScalablePool is a pool the autoscaler can resize.
type ScalablePool interface {
	Pool
	Bounds() PoolBounds
	Resize(n int) int
	TakeWindow() PoolWindow
}

// BoundedPool keeps at most capacity concurrent instances; idle ones are capped by MaxIdle (fixed pools) or
// by the capacity (autoscaled pools, so grown instances stay warm). Waiters are served in arrival order.
type BoundedPool struct {
	program   Program
	registry  *host.Registry
	opts      PoolOptions
	mu        sync.Mutex
	capacity  int
	inUse     int
	waiters   []chan struct{}
	window    PoolWindow
	idle      []Instance
	size      int
	discarded uint64
	cold      uint64
	closed    bool
	reserved  int
	memory    map[Instance]int64
	calls     atomic.Uint64
	cpu       atomic.Int64
}

// NewBoundedPool creates the pool, reserves its starting slots and warms opts.Warm instances (fails on load errors).
func NewBoundedPool(ctx context.Context, p Program, registry *host.Registry, opts PoolOptions) (*BoundedPool, error) {
	opts.Max = opts.Scale.Clamp(max(opts.Max, int(PoolParallelismDefault)))
	opts.Warm = min(opts.Warm, opts.Max)
	pool := &BoundedPool{program: p, registry: registry, opts: opts, capacity: opts.Max, memory: map[Instance]int64{}}
	pool.reserveStart()
	for range opts.Warm {
		inst, err := p.NewInstance(ctx, registry)
		if err != nil {
			_ = pool.Close(ctx)
			return nil, err
		}
		pool.idle = append(pool.idle, inst)
		pool.size++
		pool.memory[inst] = inst.MemoryBytes()
	}
	return pool, nil
}

// Acquire returns an idle instance or creates one; it waits for a free slot until ctx ends.
func (p *BoundedPool) Acquire(ctx context.Context) (Instance, error) {
	if err := p.takeSlot(ctx); err != nil {
		return nil, err
	}
	inst, err := p.takeIdle()
	if inst != nil || err != nil {
		p.releaseSlotOnError(err)
		return inst, err
	}
	inst, err = p.program.NewInstance(ctx, p.registry)
	if err != nil {
		p.releaseSlot()
		return nil, err
	}
	p.mu.Lock()
	p.size++
	p.cold++
	p.memory[inst] = inst.MemoryBytes()
	p.mu.Unlock()
	return inst, nil
}

// takeSlot takes a free slot at once or queues for one; ctx ending first is a retryable exhaustion.
func (p *BoundedPool) takeSlot(ctx context.Context) error {
	p.mu.Lock()
	if p.inUse < p.capacity && len(p.waiters) == 0 {
		p.grantLocked()
		p.mu.Unlock()
		return nil
	}
	ch := make(chan struct{})
	p.waiters = append(p.waiters, ch)
	p.mu.Unlock()
	began := time.Now()
	select {
	case <-ch:
		p.mu.Lock()
		p.window.Waited++
		p.window.WaitTotal += time.Since(began)
		p.mu.Unlock()
		return nil
	case <-ctx.Done():
	}
	p.abandonWait(ch)
	return &Error{Kind: ErrorInternal, Message: string(MsgPoolExhausted), Retryable: true}
}

// abandonWait leaves the queue, handing back a slot that was granted meanwhile.
func (p *BoundedPool) abandonWait(ch chan struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, w := range p.waiters {
		if w == ch {
			p.waiters = append(p.waiters[:i], p.waiters[i+1:]...)
			return
		}
	}
	p.releaseSlotLocked()
}

// grantLocked takes one slot and updates the window.
func (p *BoundedPool) grantLocked() {
	p.inUse++
	p.window.Calls++
	p.window.PeakInUse = max(p.window.PeakInUse, p.inUse)
}

// releaseSlotLocked frees one slot and grants it to the oldest waiter.
func (p *BoundedPool) releaseSlotLocked() {
	p.inUse = max(p.inUse-1, 0)
	p.wakeLocked()
}

// wakeLocked grants free slots to queued waiters in arrival order while capacity allows.
func (p *BoundedPool) wakeLocked() {
	for len(p.waiters) > 0 && p.inUse < p.capacity {
		ch := p.waiters[0]
		p.waiters = p.waiters[1:]
		p.grantLocked()
		close(ch)
	}
}

func (p *BoundedPool) releaseSlot() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.releaseSlotLocked()
}

func (p *BoundedPool) takeIdle() (Instance, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, &Error{Kind: ErrorInternal, Message: string(MsgPoolClosed)}
	}
	n := len(p.idle)
	if n == 0 {
		return nil, nil
	}
	inst := p.idle[n-1]
	p.idle = p.idle[:n-1]
	return inst, nil
}

func (p *BoundedPool) releaseSlotOnError(err error) {
	if err != nil {
		p.releaseSlot()
	}
}

// idleCapLocked is how many idle instances the pool keeps.
func (p *BoundedPool) idleCapLocked() int {
	if p.opts.Scale.Autoscaled() {
		return p.capacity
	}
	return p.opts.MaxIdle
}

// Release returns a healthy instance; surplus (above the idle cap or a lowered capacity) or post-close
// instances are closed.
func (p *BoundedPool) Release(i Instance) {
	p.mu.Lock()
	keep := !p.closed && p.size <= p.capacity && len(p.idle) < p.idleCapLocked()
	if keep {
		p.idle = append(p.idle, i)
		p.memory[i] = i.MemoryBytes()
	} else {
		p.size--
		delete(p.memory, i)
	}
	p.releaseSlotLocked()
	p.mu.Unlock()
	if !keep {
		_ = i.Close(context.Background())
	}
}

// Discard closes an instance that timed out, ran out of memory or trapped.
func (p *BoundedPool) Discard(ctx context.Context, i Instance) {
	_ = i.Close(ctx)
	p.mu.Lock()
	p.size--
	p.discarded++
	delete(p.memory, i)
	p.releaseSlotLocked()
	p.mu.Unlock()
}

// Warm creates idle instances until the pool holds target live instances (bounded by the capacity and
// the idle cap) and returns how many it created; instances are built outside the lock.
func (p *BoundedPool) Warm(ctx context.Context, target int) (int, error) {
	created := 0
	for p.needsWarm(target) {
		inst, err := p.program.NewInstance(ctx, p.registry)
		if err != nil {
			return created, err
		}
		if !p.addIdle(inst) {
			_ = inst.Close(ctx)
			return created, nil
		}
		created++
	}
	return created, nil
}

func (p *BoundedPool) needsWarm(target int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.closed && p.size < min(target, p.capacity) && len(p.idle) < p.idleCapLocked()
}

// addIdle parks a warmed instance unless the pool closed or filled up meanwhile.
func (p *BoundedPool) addIdle(inst Instance) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.size >= p.capacity || len(p.idle) >= p.idleCapLocked() {
		return false
	}
	p.idle = append(p.idle, inst)
	p.size++
	p.memory[inst] = inst.MemoryBytes()
	return true
}

// Stats reports pool sizes for metrics.
func (p *BoundedPool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := PoolStats{StepPath: p.program.Source().StepPath, Size: p.size, Idle: len(p.idle),
		InUse: p.size - len(p.idle), Capacity: p.capacity, Discarded: p.discarded, ColdStarts: p.cold,
		Calls: p.calls.Load(), CPUTime: time.Duration(p.cpu.Load())}
	for _, bytes := range p.memory {
		st.MemoryBytes += bytes
	}
	return st
}

// Charge accounts one call's guest execution time.
func (p *BoundedPool) Charge(cpu time.Duration) {
	p.calls.Add(1)
	p.cpu.Add(int64(cpu))
}

// Close stops handing out instances, closes idle ones and returns the reserved memory; in-use ones close on release.
func (p *BoundedPool) Close(ctx context.Context) error {
	p.mu.Lock()
	idle := p.idle
	p.idle, p.closed = nil, true
	p.size -= len(idle)
	for _, inst := range idle {
		delete(p.memory, inst)
	}
	p.freeLocked(p.reserved)
	p.mu.Unlock()
	for _, inst := range idle {
		_ = inst.Close(ctx)
	}
	return nil
}
