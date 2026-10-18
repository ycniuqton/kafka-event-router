package dlq

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ResolutionState is how a dead letter was resolved.
type ResolutionState string

const (
	StateRetried   ResolutionState = "retried"
	StateFiltered  ResolutionState = "filtered"
	StateDiscarded ResolutionState = "discarded"
)

// Destination is where a retried event was produced.
type Destination struct {
	Topic     string `json:"topic"`
	Partition int32  `json:"partition"`
	Offset    int64  `json:"offset"`
}

// Resolution is one entry of the resolved-state topic.
type Resolution struct {
	Ref         Ref             `json:"ref"`
	RouteID     string          `json:"routeId"`
	State       ResolutionState `json:"state"`
	By          string          `json:"by"`
	At          time.Time       `json:"at"`
	Destination *Destination    `json:"destination,omitempty"`
	JobID       string          `json:"jobId,omitempty"`
}

// Resolutions are looked-up resolutions by ref.
type Resolutions map[Ref]Resolution

// Of returns the resolution of ref, or nil when it is unresolved.
func (r Resolutions) Of(ref Ref) *Resolution {
	v, ok := r[ref]
	if !ok {
		return nil
	}
	return &v
}

// StateStore keeps resolved state per DLQ connection (design §2.2).
type StateStore interface {
	Lookup(ctx context.Context, conn, routeID string, refs []Ref) (map[Ref]Resolution, error)
	Mark(ctx context.Context, conn string, rs []Resolution) error
	Clear(ctx context.Context, conn, routeID string, refs []Ref) error
	CountResolved(ctx context.Context, conn, routeID string, ranges []PartitionRange) (int, error)
	Expired(ctx context.Context, conn, routeID string, ranges []PartitionRange) ([]Ref, error)
}

// StateBackend selects the StateStore implementation (ROUTER_DLQ_STATE_BACKEND).
type StateBackend string

const (
	StateBackendKafka  StateBackend = "kafka"
	StateBackendMemory StateBackend = "memory"
)

// StateFactory builds a StateStore.
type StateFactory func() StateStore

// StateRegistry selects the configured state backend.
type StateRegistry struct {
	factories map[StateBackend]StateFactory
}

// NewStateRegistry returns an empty registry.
func NewStateRegistry() *StateRegistry {
	return &StateRegistry{factories: map[StateBackend]StateFactory{}}
}

// Register adds a backend.
func (r *StateRegistry) Register(b StateBackend, f StateFactory) { r.factories[b] = f }

// Build returns the store of backend b.
func (r *StateRegistry) Build(b StateBackend) (StateStore, error) {
	f, ok := r.factories[b]
	if !ok {
		return nil, fmt.Errorf("dlq: no state backend %q", b)
	}
	return f(), nil
}

// StateKeySeparator joins the parts of a state key.
type StateKeySeparator string

const stateKeySep StateKeySeparator = ":"

// stateKeyParts is the number of parts of a state key.
type stateKeyParts int

const stateKeyLen stateKeyParts = 4

// StateKey codes the compacted-topic key "routeID:topic:partition:offset".
type StateKey struct{}

// Of returns the key of a ref of route.
func (StateKey) Of(routeID string, r Ref) string {
	return strings.Join([]string{routeID, r.Topic, strconv.FormatInt(int64(r.Partition), int(decimal)), strconv.FormatInt(r.Offset, int(decimal))}, string(stateKeySep))
}

// Parse returns route and ref of a key.
func (StateKey) Parse(key string) (string, Ref, bool) {
	parts := strings.Split(key, string(stateKeySep))
	if len(parts) != int(stateKeyLen) {
		return "", Ref{}, false
	}
	p, perr := strconv.ParseInt(parts[2], int(decimal), int(bits32))
	o, oerr := strconv.ParseInt(parts[3], int(decimal), int(bits64))
	if perr != nil || oerr != nil {
		return "", Ref{}, false
	}
	return parts[0], Ref{Topic: parts[1], Partition: int32(p), Offset: o}, true
}

// StateIndex is the in-memory materialization of resolved state of one connection.
type StateIndex struct {
	mu      sync.RWMutex
	byRoute map[string]map[Ref]Resolution
}

// NewStateIndex returns an empty index.
func NewStateIndex() *StateIndex {
	return &StateIndex{byRoute: map[string]map[Ref]Resolution{}}
}

// Put stores a resolution.
func (x *StateIndex) Put(r Resolution) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.byRoute[r.RouteID] == nil {
		x.byRoute[r.RouteID] = map[Ref]Resolution{}
	}
	x.byRoute[r.RouteID][r.Ref] = r
}

// Delete removes a resolution (tombstone).
func (x *StateIndex) Delete(routeID string, ref Ref) {
	x.mu.Lock()
	defer x.mu.Unlock()
	delete(x.byRoute[routeID], ref)
}

// Apply applies one compacted-topic record; nil value is a tombstone.
func (x *StateIndex) Apply(key, value []byte) {
	if value == nil {
		if route, ref, ok := (StateKey{}).Parse(string(key)); ok {
			x.Delete(route, ref)
		}
		return
	}
	var r Resolution
	if json.Unmarshal(value, &r) == nil && r.RouteID != "" {
		x.Put(r)
	}
}

// Lookup returns the resolutions of refs.
func (x *StateIndex) Lookup(routeID string, refs []Ref) map[Ref]Resolution {
	x.mu.RLock()
	defer x.mu.RUnlock()
	out := map[Ref]Resolution{}
	for _, ref := range refs {
		if r, ok := x.byRoute[routeID][ref]; ok {
			out[ref] = r
		}
	}
	return out
}

// Count returns resolutions of route inside the ranges [Low, High).
func (x *StateIndex) Count(routeID string, ranges []PartitionRange) int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	byKey := map[PartitionKey]PartitionRange{}
	for _, r := range ranges {
		byKey[r.Key()] = r
	}
	n := 0
	for ref := range x.byRoute[routeID] {
		pr, ok := byKey[KeyOf(ref.Topic, ref.Partition)]
		if ok && ref.Offset >= pr.Low && ref.Offset < pr.High {
			n++
		}
	}
	return n
}

// Below returns the refs of route whose offset fell below the low watermark of their partition
// (deleted by DLQ retention); refs of partitions outside ranges are kept.
func (x *StateIndex) Below(routeID string, ranges []PartitionRange) []Ref {
	x.mu.RLock()
	defer x.mu.RUnlock()
	low := map[PartitionKey]int64{}
	for _, r := range ranges {
		low[r.Key()] = r.Low
	}
	out := []Ref{}
	for ref := range x.byRoute[routeID] {
		if l, ok := low[KeyOf(ref.Topic, ref.Partition)]; ok && ref.Offset < l {
			out = append(out, ref)
		}
	}
	return out
}
