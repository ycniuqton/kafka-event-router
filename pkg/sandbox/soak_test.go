package sandbox_test

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/boundary"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

// SoakSetting names the env knobs of the soak test.
type SoakSetting string

const (
	soakCallsEnv  SoakSetting = "ROUTER_SANDBOX_SOAK_CALLS"
	soakDefault               = 4000
	soakWorkers               = 4
	soakHeapSlack             = 32 << 20
	soakRSSSlack              = 64 << 20
	soakTimeoutMs             = 1000
)

const soakCode = `function transform(e, ctx) {
  const items = [];
  for (let i = 0; i < 5; i++) items.push({ i, h: ctx.hash(e.offset + ':' + i), r: Math.random() });
  ctx.log('event', e.offset, ctx.now());
  if (e.offset % 7 === 0) return null;
  if (e.offset % 5 === 0) return [{ value: items.slice(0, 2) }, { value: { big: 'x'.repeat(4096) } }];
  return { key: ctx.mask.last4(String(e.offset)), value: { items, country: ctx.lookup('countries', 'DE') } };
}`

// TestSoakMemoryStable (JS-207) runs ROUTER_SANDBOX_SOAK_CALLS calls (10M in the nightly soak,
// a short smoke by default) through one pool and checks that instance memory, Go heap and RSS
// stop growing after warm-up.
func TestSoakMemoryStable(t *testing.T) {
	calls := soakCalls(t)
	m := sandboxtest.NewManager(t)
	scope := sandboxtest.Scope(soakWorkers)
	rs, err := m.Load(context.Background(), scope, []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform, Code: soakCode}}, sandbox.RouteLimits{JSTimeoutMs: soakTimeoutMs})
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close(context.Background())
	mod, _ := rs.Module("/s")
	run := soakRunner{mod: mod}
	run.calls(t, soakWorkers, min(calls/10, 2000))
	base := sampleMemory(rs)
	start := time.Now()
	run.calls(t, soakWorkers, calls)
	end := sampleMemory(rs)
	t.Logf("%d calls in %v: instances %d→%d bytes, heap %d→%d, rss %d→%d, discarded %d, timeouts %d",
		run.done.Load(), time.Since(start), base.instances, end.instances, base.heap, end.heap, base.rss, end.rss, end.discarded, run.timeouts.Load())
	if end.instances > base.instances*11/10 {
		t.Fatalf("instance memory grew from %d to %d bytes", base.instances, end.instances)
	}
	if end.heap > base.heap+soakHeapSlack || (base.rss > 0 && end.rss > base.rss+soakRSSSlack) {
		t.Fatalf("process memory grew: heap %d→%d rss %d→%d", base.heap, end.heap, base.rss, end.rss)
	}
}

func soakCalls(t *testing.T) int {
	raw, ok := os.LookupEnv(string(soakCallsEnv))
	if !ok {
		if testing.Short() {
			t.Skip("soak smoke skipped in -short")
		}
		return soakDefault
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		t.Fatalf("%s must be a positive integer", soakCallsEnv)
	}
	return n
}

// soakRunner spreads calls over workers sharing one module.
type soakRunner struct {
	mod      sandbox.Module
	next     atomic.Int64
	done     atomic.Int64
	timeouts atomic.Int64
}

func (r *soakRunner) calls(t *testing.T, workers, n int) {
	var wg sync.WaitGroup
	end := r.next.Load() + int64(n)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for off := r.next.Add(1); off <= end; off = r.next.Add(1) {
				ev := boundary.GuestEvent{Topic: "soak", Offset: off, Value: map[string]any{"n": off}}
				_, err := r.mod.Call(context.Background(), sandbox.Invocation{Event: ev, Now: time.Unix(off, 0)})
				if se, ok := sandbox.AsError(err); ok && se.Kind == sandbox.ErrorTimeout {
					r.timeouts.Add(1)
				} else if err != nil {
					t.Error(err)
					return
				}
				r.done.Add(1)
			}
		}()
	}
	wg.Wait()
}

// memorySample is one reading of sandbox and process memory.
type memorySample struct {
	instances int64
	heap      uint64
	rss       int64
	discarded uint64
}

func sampleMemory(rs sandbox.RouteSandbox) memorySample {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	s := memorySample{heap: ms.HeapAlloc, rss: processRSS()}
	for _, st := range rs.Stats() {
		s.instances += st.MemoryBytes
		s.discarded += st.Discarded
	}
	return s
}

// processRSS reads the resident set size from /proc (0 where unavailable).
func processRSS() int64 {
	raw, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 2 {
		return 0
	}
	pages, _ := strconv.ParseInt(fields[1], 10, 64)
	return pages * int64(os.Getpagesize())
}
