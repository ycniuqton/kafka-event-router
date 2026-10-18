package livetail_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/livetail"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// countingSource counts how many records the tap asked it to build.
type countingSource struct {
	built *atomic.Int64
}

func (s countingSource) TailRecord(livetail.TailLimits) livetail.TailRecord {
	s.built.Add(1)
	return livetail.TailRecord{At: time.Now(), Outcome: livetail.OutcomeSent}
}

// dropCounter records drops per reason.
type dropCounter struct {
	mu    sync.Mutex
	drops map[livetail.DropReason]int
}

func (d *dropCounter) Dropped(_ string, r livetail.DropReason) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.drops[r]++
}

func newRedis(t *testing.T) (*miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return mr, client
}

func waitActive(t *testing.T, tap livetail.Tap, route string, want bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if tap.Active(route) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("Active(%s) never became %v", route, want)
}

func fastLimits() livetail.TailLimits {
	l := livetail.DefaultTailLimits()
	l.FlagCacheTTL = 10 * time.Millisecond
	return l
}

func TestRedisTapWithoutFlagCostsNothing(t *testing.T) {
	mr, client := newRedis(t)
	tap := livetail.NewRedisTap(client, fastLimits(), nil, quiet)
	var built atomic.Int64
	for i := 0; i < 50; i++ {
		if tap.Active("r1") {
			tap.Offer("r1", countingSource{built: &built})
		}
		time.Sleep(time.Millisecond)
	}
	if built.Load() != 0 {
		t.Fatalf("built %d records without a flag", built.Load())
	}
	if mr.Exists(livetail.TailKey{RouteID: "r1"}.Stream()) {
		t.Fatal("stream written without a flag")
	}
}

func TestRedisTapRateLimitsPerRoute(t *testing.T) {
	mr, client := newRedis(t)
	drops := &dropCounter{drops: map[livetail.DropReason]int{}}
	limits := fastLimits()
	tap := livetail.NewRedisTap(client, limits, drops, quiet)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tap.Run(ctx)
	mr.Set(livetail.TailKey{RouteID: "r1"}.Flag(), "1")
	waitActive(t, tap, "r1", true)
	var built atomic.Int64
	for i := 0; i < 100; i++ {
		tap.Offer("r1", countingSource{built: &built})
	}
	key := livetail.TailKey{RouteID: "r1"}.Stream()
	waitLen(t, client, key, int64(limits.Burst)+1)
	if got := built.Load(); got > int64(limits.Burst)+1 {
		t.Fatalf("built %d records, want ≤ burst", got)
	}
	if drops.drops[livetail.DropRate] < 100-limits.Burst-1 {
		t.Fatalf("rate drops = %d", drops.drops[livetail.DropRate])
	}
	if ttl := mr.TTL(key); ttl <= 0 || ttl > limits.StreamTTL {
		t.Fatalf("stream ttl %v", ttl)
	}
	kinds := streamKinds(t, client, key)
	if kinds[livetail.EntryThrottled] != 1 || kinds[livetail.EntryRecord] < limits.Burst {
		t.Fatalf("stream kinds %v", kinds)
	}
}

func waitLen(t *testing.T, c redis.UniversalClient, key string, want int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if n, _ := c.XLen(context.Background(), key).Result(); n >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	n, _ := c.XLen(context.Background(), key).Result()
	t.Fatalf("stream length %d, want %d", n, want)
}

func streamKinds(t *testing.T, c redis.UniversalClient, key string) map[livetail.EntryKind]int {
	t.Helper()
	msgs, err := c.XRange(context.Background(), key, "-", "+").Result()
	if err != nil {
		t.Fatal(err)
	}
	out := map[livetail.EntryKind]int{}
	for _, m := range msgs {
		out[livetail.EntryDecoder{}.Decode(m).Kind]++
	}
	return out
}

func TestTapManagerSelection(t *testing.T) {
	if _, err := livetail.NewTapManager(false).Select(livetail.TapRedis, livetail.TapDeps{}); err == nil {
		t.Fatal("redis tap offered without a client")
	}
	tap, err := livetail.NewTapManager(false).Select(livetail.TapNoop, livetail.TapDeps{})
	if err != nil || tap.Active("x") {
		t.Fatalf("noop tap: %v", err)
	}
	if _, err := livetail.NewTapManager(true).Select("bogus", livetail.TapDeps{}); err == nil {
		t.Fatal("unknown tap accepted")
	}
}

func TestSettingsLoader(t *testing.T) {
	env := map[string]string{string(livetail.EnvLiveTailTap): "noop", string(livetail.EnvLiveTailRate): "5"}
	s, err := livetail.SettingsLoader{Lookup: func(k string) (string, bool) { v, ok := env[k]; return v, ok }}.Load()
	if err != nil || s.Tap != livetail.TapNoop || s.Limits.RatePerSec != 5 || s.Limits.Burst != 5 {
		t.Fatalf("settings %+v %v", s, err)
	}
	if (livetail.TailKey{}).RouteOf(livetail.TailKey{RouteID: "abc"}.Flag()) != "abc" {
		t.Fatal("RouteOf")
	}
}

// TestRedisTapSweepsKnownFlags: a quiet route is tailed from its first record after the flag appears
// (the flag of a route seen before is refreshed in the background, not only when records arrive).
func TestRedisTapSweepsKnownFlags(t *testing.T) {
	mr, client := newRedis(t)
	tap := livetail.NewRedisTap(client, fastLimits(), nil, quiet)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tap.Run(ctx)
	waitActive(t, tap, "quiet", false)
	time.Sleep(50 * time.Millisecond)
	mr.Set(livetail.TailKey{RouteID: "quiet"}.Flag(), "1")
	time.Sleep(200 * time.Millisecond)
	if !tap.Active("quiet") {
		t.Fatal("first record after the flag appeared was not tailed")
	}
}
