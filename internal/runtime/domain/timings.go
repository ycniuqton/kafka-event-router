package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/dlq/record"
)

// Timing is a named runtime duration (design §2.1 Timings).
type Timing time.Duration

const (
	CommitInterval     Timing = Timing(time.Second)
	BackoffMin         Timing = Timing(100 * time.Millisecond)
	BackoffMax         Timing = Timing(30 * time.Second)
	LeaseTTL           Timing = Timing(30 * time.Second)
	HeartbeatInterval  Timing = Timing(10 * time.Second)
	ResyncInterval     Timing = Timing(15 * time.Second)
	AutoRollbackWindow Timing = Timing(60 * time.Second)
	DrainTimeout       Timing = Timing(30 * time.Second)
	StopGracePeriod    Timing = Timing(30 * time.Second)
	SchedulerTick      Timing = Timing(5 * time.Second)
	ChangeDebounce     Timing = Timing(200 * time.Millisecond)
	StreamKeepAlive    Timing = Timing(15 * time.Second)
	DrainPoll          Timing = Timing(10 * time.Millisecond)
	LagProbeInterval   Timing = Timing(10 * time.Second)
	TransactionBatch   Timing = Timing(30 * time.Second)
	ReplayGroupWait    Timing = Timing(60 * time.Second)
	RunnerRecovery     Timing = Timing(2 * time.Second)
)

// D returns the timing as a time.Duration.
func (t Timing) D() time.Duration { return time.Duration(t) }

// Timings exposes the runtime timing constants as one holder (tests may shrink them).
type Timings struct {
	Commit       time.Duration
	BackoffMin   time.Duration
	BackoffMax   time.Duration
	LeaseTTL     time.Duration
	Heartbeat    time.Duration
	Resync       time.Duration
	AutoRollback time.Duration
	Drain        time.Duration
	StopGrace    time.Duration
	Scheduler    time.Duration
	LagProbe     time.Duration
	Batch        time.Duration
	ReplayWait   time.Duration
}

// DefaultTimings returns the production timings.
func DefaultTimings() Timings {
	return Timings{
		Commit: CommitInterval.D(), BackoffMin: BackoffMin.D(), BackoffMax: BackoffMax.D(), LeaseTTL: LeaseTTL.D(),
		Heartbeat: HeartbeatInterval.D(), Resync: ResyncInterval.D(), AutoRollback: AutoRollbackWindow.D(),
		Drain: DrainTimeout.D(), StopGrace: StopGracePeriod.D(), Scheduler: SchedulerTick.D(), LagProbe: LagProbeInterval.D(),
		Batch: TransactionBatch.D(), ReplayWait: ReplayGroupWait.D(),
	}
}

// BackoffFactor is the exponential growth factor of retry delays.
type BackoffFactor float64

const DefaultBackoffFactor BackoffFactor = 2

// JitterRatio is the ± fraction applied to every retry delay.
type JitterRatio float64

const DefaultJitter JitterRatio = 0.2

// LoadWeight holds the scheduler load-score weights (design §2.7).
type LoadWeight float64

const (
	RouteBaseWeight         LoadWeight = 1
	PartitionWeightDivisor  LoadWeight = 10
	DefaultWorkerCapacity   LoadWeight = 100
	MinimumCapacityForScore LoadWeight = 1
)

// LoadWeights computes the load of routes and workers.
type LoadWeights struct{}

// Route returns the weight of one replica reading partitions source partitions.
func (LoadWeights) Route(partitions int) float64 {
	return float64(RouteBaseWeight) + float64(partitions)/float64(PartitionWeightDivisor)
}

// Score returns the load score of a worker (sum of weights ÷ capacity).
func (LoadWeights) Score(used float64, capacity int) float64 {
	c := float64(capacity)
	if c < float64(MinimumCapacityForScore) {
		c = float64(MinimumCapacityForScore)
	}
	return used / c
}

// RetentionDays is the DLQ topic retention (decision: 7 days).
type RetentionDays int

const DLQRetentionDays RetentionDays = 7

// Millis returns the retention in milliseconds.
func (d RetentionDays) Millis() int64 {
	return int64(time.Duration(d) * 24 * time.Hour / time.Millisecond)
}

// TopicNaming derives runtime-owned topic names.
type TopicNaming struct{}

// DLQDefault returns the default DLQ topic of a route; the naming belongs to the dead-letters record contract.
func (TopicNaming) DLQDefault(routeName string) string {
	return record.TopicNaming{}.Default(routeName)
}

// GroupPattern is the default consumer group naming pattern when a route sets none.
type GroupPattern string

const DefaultGroupPattern GroupPattern = "router.%s"

// GroupNaming derives the default consumer group of a route.
type GroupNaming struct{}

// Default returns "router.<routeID>".
func (GroupNaming) Default(routeID string) string {
	return fmt.Sprintf(string(DefaultGroupPattern), routeID)
}

// Of returns the configured source.consumerGroup, or the route's default group when none is set.
func (n GroupNaming) Of(routeID string, configured *string) string {
	if configured != nil && *configured != "" {
		return *configured
	}
	return n.Default(routeID)
}

// TransactionalIDPattern names the transactional producer of one exactly_once runner instance.
type TransactionalIDPattern string

const DefaultTransactionalIDPattern TransactionalIDPattern = "router.%s.%s"

// TransactionalIDs derives transactional ids; each runner instance gets its own (KIP-447 fences zombies
// through the consumer group generation, not through a shared id).
type TransactionalIDs struct{}

// New returns "router.<routeID>.<random>".
func (TransactionalIDs) New(routeID string) string {
	return fmt.Sprintf(string(DefaultTransactionalIDPattern), routeID, uuid.NewString())
}

// ReplicaDefault is the replica count used when a route row has none.
type ReplicaDefault int

const DefaultReplicas ReplicaDefault = 2
