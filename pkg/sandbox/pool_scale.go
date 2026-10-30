package sandbox

import "context"

// slotBytes is the memory one slot reserves from the ceiling: the program's per-instance cap.
func (p *BoundedPool) slotBytes() int64 { return p.program.Limits().MemoryBytes }

// reserveStart charges the starting slots to the ceiling; they are never refused.
func (p *BoundedPool) reserveStart() {
	if p.opts.Ceiling == nil {
		return
	}
	p.opts.Ceiling.Force(int64(p.capacity) * p.slotBytes())
	p.reserved = p.capacity
}

// freeLocked returns n reserved slots to the ceiling.
func (p *BoundedPool) freeLocked(n int) {
	if p.opts.Ceiling == nil || n <= 0 {
		return
	}
	p.opts.Ceiling.Free(int64(n) * p.slotBytes())
	p.reserved -= n
}

// reserveLocked takes up to n more slots from the ceiling and returns how many fit.
func (p *BoundedPool) reserveLocked(n int) int {
	if p.opts.Ceiling == nil {
		return n
	}
	got := 0
	for got < n && p.opts.Ceiling.TryReserve(p.slotBytes()) {
		got++
	}
	p.reserved += got
	return got
}

// Bounds returns the autoscaling bounds (zero for fixed pools).
func (p *BoundedPool) Bounds() PoolBounds { return p.opts.Scale }

// Resize moves the capacity toward n inside the bounds: growth takes only the slots the memory ceiling
// grants and wakes waiters; shrinking closes surplus idle instances, busy ones close when released.
// It returns the capacity now in effect.
func (p *BoundedPool) Resize(n int) int {
	p.mu.Lock()
	if p.closed {
		defer p.mu.Unlock()
		return p.capacity
	}
	target := p.opts.Scale.Clamp(max(n, 1))
	if target > p.capacity {
		p.capacity += p.reserveLocked(target - p.capacity)
		p.wakeLocked()
	}
	if target < p.capacity {
		p.freeLocked(p.capacity - target)
		p.capacity = target
	}
	surplus := p.trimIdleLocked()
	capacity := p.capacity
	p.mu.Unlock()
	for _, inst := range surplus {
		_ = inst.Close(context.Background())
	}
	return capacity
}

// trimIdleLocked removes idle instances while more instances live than the capacity allows.
func (p *BoundedPool) trimIdleLocked() []Instance {
	var out []Instance
	for p.size > p.capacity && len(p.idle) > 0 {
		inst := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]
		p.size--
		delete(p.memory, inst)
		out = append(out, inst)
	}
	return out
}

// TakeWindow returns the slot statistics since the previous call and starts a new window.
func (p *BoundedPool) TakeWindow() PoolWindow {
	p.mu.Lock()
	defer p.mu.Unlock()
	w := p.window
	w.Size = p.capacity
	p.window = PoolWindow{PeakInUse: p.inUse}
	return w
}
