package store

import (
	"context"
	"sync"
)

type commitHooksKey struct{}

// commitHooks collects callbacks that run only once the surrounding transaction committed.
type commitHooks struct {
	mu  sync.Mutex
	fns []func()
}

func (h *commitHooks) add(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fns = append(h.fns, fn)
}

func (h *commitHooks) run() {
	h.mu.Lock()
	fns := h.fns
	h.fns = nil
	h.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// AfterCommit runs fn after the transaction carried by ctx commits (never on rollback); without a
// transaction the write is already durable, so fn runs immediately.
func AfterCommit(ctx context.Context, fn func()) {
	if hooks, ok := ctx.Value(commitHooksKey{}).(*commitHooks); ok {
		hooks.add(fn)
		return
	}
	fn()
}
