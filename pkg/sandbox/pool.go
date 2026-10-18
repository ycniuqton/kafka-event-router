package sandbox

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eventrouter/router/pkg/sandbox/host"
)

// PoolOptions size one pool; Max bounds concurrent instances (route parallelism).
type PoolOptions struct {
	Max     int
	Warm    int
	MaxIdle int
}

// PoolStats is a point-in-time view of one pool; ColdStarts counts instances booted while a call
// waited (none after a pre-warm sized to the route's parallelism); MemoryBytes covers every live instance (last
// sampled size) and CPUTime is the cumulative guest execution time for capacity scoring.
type PoolStats struct {
	StepPath    string
	Size        int
	Idle        int
	InUse       int
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

// BoundedPool keeps at most Max live instances and MaxIdle idle ones.
type BoundedPool struct {
	program   Program
	registry  *host.Registry
	opts      PoolOptions
	slots     chan struct{}
	mu        sync.Mutex
	idle      []Instance
	size      int
	discarded uint64
	cold      uint64
	closed    bool
	memory    map[Instance]int64
	calls     atomic.Uint64
	cpu       atomic.Int64
}

// NewBoundedPool creates the pool and warms opts.Warm instances (fails on load errors).
func NewBoundedPool(ctx context.Context, p Program, registry *host.Registry, opts PoolOptions) (*BoundedPool, error) {
	opts.Max = max(opts.Max, int(PoolParallelismDefault))
	opts.Warm = min(opts.Warm, opts.Max)
	pool := &BoundedPool{program: p, registry: registry, opts: opts, slots: make(chan struct{}, opts.Max), memory: map[Instance]int64{}}
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
	select {
	case p.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, &Error{Kind: ErrorInternal, Message: string(MsgPoolExhausted), Retryable: true}
	}
	inst, err := p.takeIdle()
	if inst != nil || err != nil {
		p.releaseSlotOnError(err)
		return inst, err
	}
	inst, err = p.program.NewInstance(ctx, p.registry)
	if err != nil {
		<-p.slots
		return nil, err
	}
	p.mu.Lock()
	p.size++
	p.cold++
	p.memory[inst] = inst.MemoryBytes()
	p.mu.Unlock()
	return inst, nil
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
		<-p.slots
	}
}

// Release returns a healthy instance; surplus or post-close instances are closed.
func (p *BoundedPool) Release(i Instance) {
	p.mu.Lock()
	keep := !p.closed && len(p.idle) < p.opts.MaxIdle
	if keep {
		p.idle = append(p.idle, i)
		p.memory[i] = i.MemoryBytes()
	} else {
		p.size--
		delete(p.memory, i)
	}
	p.mu.Unlock()
	if !keep {
		_ = i.Close(context.Background())
	}
	<-p.slots
}

// Discard closes an instance that timed out, ran out of memory or trapped.
func (p *BoundedPool) Discard(ctx context.Context, i Instance) {
	_ = i.Close(ctx)
	p.mu.Lock()
	p.size--
	p.discarded++
	delete(p.memory, i)
	p.mu.Unlock()
	<-p.slots
}

// Warm creates idle instances until the pool holds target live instances (bounded by Max and
// MaxIdle) and returns how many it created; instances are built outside the lock.
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
	return !p.closed && p.size < min(target, p.opts.Max) && len(p.idle) < p.opts.MaxIdle
}

// addIdle parks a warmed instance unless the pool closed or filled up meanwhile.
func (p *BoundedPool) addIdle(inst Instance) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.size >= p.opts.Max || len(p.idle) >= p.opts.MaxIdle {
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
		InUse: p.size - len(p.idle), Discarded: p.discarded, ColdStarts: p.cold,
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

// Close stops handing out instances and closes idle ones; in-use ones close on release.
func (p *BoundedPool) Close(ctx context.Context) error {
	p.mu.Lock()
	idle := p.idle
	p.idle, p.closed = nil, true
	p.size -= len(idle)
	for _, inst := range idle {
		delete(p.memory, inst)
	}
	p.mu.Unlock()
	for _, inst := range idle {
		_ = inst.Close(ctx)
	}
	return nil
}
