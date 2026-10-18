package sandbox_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/host"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

type fakeProgram struct {
	created atomic.Int32
	closed  atomic.Int32
	failNew bool
}

func (p *fakeProgram) Source() sandbox.Source { return sandbox.Source{StepPath: "/fake"} }
func (p *fakeProgram) Limits() sandbox.Limits { return sandbox.LimitDefault }
func (p *fakeProgram) NewInstance(context.Context, *host.Registry) (sandbox.Instance, error) {
	if p.failNew {
		return nil, errors.New("boot failed")
	}
	p.created.Add(1)
	return &fakeInstance{p: p}, nil
}

type fakeInstance struct{ p *fakeProgram }

func (i *fakeInstance) Invoke(context.Context, sandbox.GuestCall) (sandbox.GuestReply, error) {
	return sandbox.GuestReply{}, nil
}
func (i *fakeInstance) MemoryBytes() int64 { return 1 }
func (i *fakeInstance) Reusable() bool     { return true }
func (i *fakeInstance) Close(context.Context) error {
	i.p.closed.Add(1)
	return nil
}

func TestBoundedPoolLifecycle(t *testing.T) {
	ctx := context.Background()
	prog := &fakeProgram{}
	pool, err := sandbox.NewBoundedPool(ctx, prog, nil, sandbox.PoolOptions{Max: 2, Warm: 1, MaxIdle: 1})
	if err != nil || prog.created.Load() != 1 {
		t.Fatalf("warm: %v created=%d", err, prog.created.Load())
	}
	a, _ := pool.Acquire(ctx)
	b, _ := pool.Acquire(ctx)
	if st := pool.Stats(); st.Size != 2 || st.InUse != 2 {
		t.Fatalf("stats = %+v", st)
	}
	short, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	_, err = pool.Acquire(short)
	if se, ok := sandbox.AsError(err); !ok || !se.Retryable {
		t.Fatalf("exhausted pool err = %v", err)
	}
	pool.Release(a)
	pool.Release(b)
	if st := pool.Stats(); st.Size != 1 || st.Idle != 1 || prog.closed.Load() != 1 {
		t.Fatalf("after release stats = %+v closed=%d", st, prog.closed.Load())
	}
	c, _ := pool.Acquire(ctx)
	pool.Discard(ctx, c)
	if st := pool.Stats(); st.Size != 0 || st.Discarded != 1 {
		t.Fatalf("after discard stats = %+v", st)
	}
	d, _ := pool.Acquire(ctx)
	_ = pool.Close(ctx)
	pool.Release(d)
	if _, err := pool.Acquire(ctx); err == nil || prog.closed.Load() != prog.created.Load() {
		t.Fatalf("closed pool: err=%v created=%d closed=%d", err, prog.created.Load(), prog.closed.Load())
	}
}

func TestBoundedPoolWarmFailure(t *testing.T) {
	if _, err := sandbox.NewBoundedPool(context.Background(), &fakeProgram{failNew: true}, nil, sandbox.PoolOptions{Max: 1, Warm: 1}); err == nil {
		t.Fatal("warm-up failure must fail the pool")
	}
}

const isolationProbe = `function transform(event, ctx) {
  if (event.offset % 7 === 0) { while (true) {} }
  const previous = globalThis.seen;
  globalThis.seen = event.offset;
  Object.prototype.leaked = event.offset;
  return { value: { previous: previous === undefined ? null : previous, leaked: ({}).leaked === event.offset, offset: event.offset } };
}`

// TestPoolConcurrencyIsolation runs N goroutines on one module: timed-out instances are
// replaced and no call ever sees state left by another call.
func TestPoolConcurrencyIsolation(t *testing.T) {
	m := sandboxtest.NewManager(t)
	scope := sandboxtest.Scope(4)
	rs, err := m.Load(context.Background(), scope, []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform, Code: isolationProbe}}, sandbox.RouteLimits{JSTimeoutMs: 20})
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close(context.Background())
	mod, _ := rs.Module("/s")
	var wg sync.WaitGroup
	var timeouts atomic.Int32
	errs := make(chan error, 64)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				ev := typicalEvent()
				ev.Offset = int64(g*100 + i)
				res, err := mod.Call(context.Background(), sandbox.Invocation{Event: ev, Now: time.Unix(1, 0)})
				if se, ok := sandbox.AsError(err); ok && se.Kind == sandbox.ErrorTimeout {
					timeouts.Add(1)
					continue
				}
				if err != nil {
					errs <- err
					return
				}
				v := res.Outputs[0].Value.(map[string]any)
				if v["previous"] != nil || v["leaked"] != true {
					errs <- fmt.Errorf("offset %d saw leaked state %v", ev.Offset, v)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	st := rs.Stats()[0]
	if timeouts.Load() == 0 || st.Discarded < uint64(timeouts.Load()) || st.Size > scope.Parallelism {
		t.Fatalf("timeouts=%d stats=%+v", timeouts.Load(), st)
	}
}
