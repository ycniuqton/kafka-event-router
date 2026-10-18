// Package runner is the per-route processing loop on a worker: source consumer, ordered partition lanes,
// in-flight limiter, offset tracking and commits, error policies, backpressure and hot pipeline swap.
package runner

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/kafka"
)

// CompiledVersion is one route version ready to run: typed config, compiled pipeline, resolved connections.
type CompiledVersion struct {
	Ref         domain.VersionRef
	RouteName   string
	TeamID      string
	Config      *config.RouteConfig
	Pipeline    engine.Pipeline
	Connections map[string]kafka.ConnectionSpec
}

// Runner runs one route version on this worker; owned by Manager.
type Runner interface {
	Version() CompiledVersion
	State() domain.RunnerState
	Start(ctx context.Context) error
	Swap(ctx context.Context, next CompiledVersion) error
	Stop(ctx context.Context, reason domain.StopReason) error
	Status() domain.ReplicaStatus
	Halts() <-chan *failures.HaltError
	Load() float64
}

// Factory creates runners (the Manager never builds a concrete runner itself).
type Factory interface {
	New(v CompiledVersion) Runner
}

// LaneLimit bounds the per-partition queue before fetching is paused. Both marks sit above one poll so a
// single poll never pauses a lane, and resuming leaves enough queued work to hide the re-fetch of the
// records a pause strips from the client buffer.
type LaneLimit int

const (
	LaneHighWater  LaneLimit = 2048
	LaneLowWater   LaneLimit = 1024
	MaxPollRecords LaneLimit = 1000
)

// LaneByteLimit bounds the memory of one partition queue (large records pause before the record marks).
type LaneByteLimit int64

const (
	LaneHighWaterBytes LaneByteLimit = 2 << 20
	LaneLowWaterBytes  LaneByteLimit = 1 << 20
)

// SourceTiming holds timings of the route source consumer.
type SourceTiming time.Duration

// SourceFetchMaxWait caps the broker long-poll of a fetch: a resumed partition waits for the in-flight fetch
// of the idle ones, so franz-go's 5 s default would throttle a backlog to one fetch per 5 s per partition.
const SourceFetchMaxWait SourceTiming = SourceTiming(500 * time.Millisecond)

// PauseReason is why fetching of one partition is paused.
type PauseReason string

const (
	PauseQueueFull PauseReason = "queue_full"
	PauseBackoff   PauseReason = "backoff"
)
