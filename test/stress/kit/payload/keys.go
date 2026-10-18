package payload

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strconv"
)

// KeyMode names a key strategy (loadgen --keys).
type KeyMode string

const (
	KeyRoundRobin KeyMode = "roundrobin"
	KeyRandom     KeyMode = "random"
	KeyNone       KeyMode = "none"
)

// Picker chooses the stream of the next event among the streams one producer lane owns.
type Picker interface {
	Pick(owned int) int
	KeyFor(stream string) []byte
}

// keyed carries the record key = stream name behaviour shared by keyed strategies.
type keyed struct{}

// KeyFor uses the stream name as the Kafka record key.
func (keyed) KeyFor(stream string) []byte {
	return []byte(stream)
}

// roundRobin cycles through owned streams so every stream advances evenly.
type roundRobin struct {
	keyed
	next int
}

// Pick returns the next owned stream index.
func (r *roundRobin) Pick(owned int) int {
	r.next = (r.next + 1) % owned
	return r.next
}

// random picks a uniformly random owned stream.
type random struct {
	keyed
	rng *rand.Rand
}

// Pick returns a random owned stream index.
func (r *random) Pick(owned int) int {
	return r.rng.IntN(owned)
}

// unkeyed sends records without a key (sticky partitioning); streams still carry seq for completeness checks.
type unkeyed struct {
	roundRobin
}

// KeyFor returns no key.
func (unkeyed) KeyFor(string) []byte {
	return nil
}

// KeyManager builds the configured key strategy (registry, no switch on concrete types).
type KeyManager struct{}

var keyFactories = map[KeyMode]func(seed uint64) Picker{
	KeyRoundRobin: func(uint64) Picker { return &roundRobin{} },
	KeyRandom:     func(seed uint64) Picker { return &random{rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))} },
	KeyNone:       func(uint64) Picker { return &unkeyed{} },
}

// New returns a picker for mode, seeded per producer lane.
func (KeyManager) New(mode KeyMode, seed uint64) (Picker, error) {
	f, ok := keyFactories[mode]
	if !ok {
		return nil, fmt.Errorf("unknown key mode %q (known: %v)", mode, KeyManager{}.Modes())
	}
	return f(seed), nil
}

// Modes lists the registered key modes.
func (KeyManager) Modes() []string {
	out := make([]string, 0, len(keyFactories))
	for m := range keyFactories {
		out = append(out, string(m))
	}
	sort.Strings(out)
	return out
}

// Streams partitions stream names between producer lanes: lane w owns streams w, w+lanes, w+2·lanes, ...
type Streams struct {
	Prefix string
	Total  int
	Lanes  int
}

// Owned returns the stream names of one lane (at least one).
func (s Streams) Owned(lane int) []string {
	var out []string
	for i := lane; i < max(s.Total, s.Lanes); i += s.Lanes {
		out = append(out, s.Prefix+"-"+strconv.Itoa(i))
	}
	return out
}
