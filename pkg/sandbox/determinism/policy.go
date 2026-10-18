package determinism

import (
	"context"
	"encoding/binary"
	"hash/fnv"
	"time"
)

// SeedPolicy derives the per-event Math.random seed: fnv64a(route, topic, partition, offset).
// It is independent of the route version so saved test cases stay green across versions.
type SeedPolicy struct{}

// Seed returns the seed of one event.
func (SeedPolicy) Seed(routeID, topic string, partition int32, offset int64) uint64 {
	h := fnv.New64a()
	h.Write([]byte(routeID))
	h.Write([]byte{0})
	h.Write([]byte(topic))
	buf := binary.BigEndian.AppendUint32(nil, uint32(partition))
	h.Write(binary.BigEndian.AppendUint64(buf, uint64(offset)))
	return h.Sum64()
}

// sfcConstant holds the sfc32 seeding constants shared with the prelude PRNG.
type sfcConstant uint32

const (
	sfcGolden       sfcConstant = 0x9e3779b9
	sfcWarmupRounds sfcConstant = 12
	sfcShiftB       sfcConstant = 9
	sfcShiftC       sfcConstant = 3
	sfcRotate       sfcConstant = 21
	sfcWordBits     sfcConstant = 32
)

// RandomState is the warmed-up sfc32 state the guest Math.random continues from.
type RandomState [4]uint32

// State seeds sfc32 with seed (hi, lo, hi^golden, 1) and runs the warm-up rounds.
func (SeedPolicy) State(seed uint64) RandomState {
	hi, lo := uint32(seed>>sfcWordBits), uint32(seed)
	s := RandomState{hi, lo, hi ^ uint32(sfcGolden), 1}
	for range sfcWarmupRounds {
		s.Next()
	}
	return s
}

// Next advances the generator and returns the next 32-bit output (same as the prelude).
func (s *RandomState) Next() uint32 {
	a, b, c, d := s[0], s[1], s[2], s[3]
	t := a + b + d
	d++
	a = b ^ (b >> sfcShiftB)
	b = c + (c << sfcShiftC)
	c = (c<<sfcRotate | c>>(sfcWordBits-sfcRotate)) + t
	*s = RandomState{a, b, c, d}
	return t
}

// FrozenClock is the processing time of one event, identical for every read during the call.
type FrozenClock struct {
	at time.Time
}

// Freeze captures t (UTC, millisecond precision as seen by JS Date).
func Freeze(t time.Time) FrozenClock {
	return FrozenClock{at: t.UTC().Truncate(time.Millisecond)}
}

// Now returns the frozen time.
func (c FrozenClock) Now() time.Time { return c.at }

// UnixMilli returns the frozen time as epoch milliseconds (Date.now()).
func (c FrozenClock) UnixMilli() int64 { return c.at.UnixMilli() }

// UnixNano returns the frozen time for the WASI clock.
func (c FrozenClock) UnixNano() int64 { return c.at.UnixNano() }

// Pass tells whether a record is processed for the first time or re-read by a replay.
type Pass string

const (
	PassOriginal Pass = "original"
	PassReplay   Pass = "replay"
)

// EventRef identifies a source record independently of route version, consumer group and pass.
type EventRef struct {
	RouteID   string
	Topic     string
	Partition int32
	Offset    int64
}

// Inputs are the determinism inputs of one call.
type Inputs struct {
	Seed uint64
	Now  time.Time
	Pass Pass
}

// Policy is the base of the per-event determinism rules (Math.random seed and frozen clock).
type Policy interface {
	Inputs(ref EventRef, pass Pass, processedAt time.Time) Inputs
}

// ProcessingPolicy applies one rule to every pass: the seed comes from the record (SeedPolicy) and
// ctx.now() is the processing time, so a replay draws the same Math.random values as the original
// run and sees the replay's processing time (design §10).
type ProcessingPolicy struct {
	Seeds SeedPolicy
}

func (p ProcessingPolicy) Inputs(ref EventRef, pass Pass, processedAt time.Time) Inputs {
	if pass == "" {
		pass = PassOriginal
	}
	return Inputs{Seed: p.Seeds.Seed(ref.RouteID, ref.Topic, ref.Partition, ref.Offset), Now: processedAt, Pass: pass}
}

// passKey is the context key of a Pass.
type passKey struct{}

// PassContext carries the pass through callers that only forward a context (route-runtime marks
// records of a replay window).
type PassContext struct{}

// With returns ctx carrying pass.
func (PassContext) With(ctx context.Context, pass Pass) context.Context {
	return context.WithValue(ctx, passKey{}, pass)
}

// From returns the pass carried by ctx, PassOriginal when none.
func (PassContext) From(ctx context.Context) Pass {
	if p, ok := ctx.Value(passKey{}).(Pass); ok {
		return p
	}
	return PassOriginal
}
