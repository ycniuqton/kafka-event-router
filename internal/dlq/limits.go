package dlq

import "time"

// Limit holds the count and size bounds of the feature (design §2.2).
type Limit int

const (
	DefaultPage       Limit = 50
	MaxPage           Limit = 200
	MaxScanBytes      Limit = 10 << 20
	MaxSyncAction     Limit = 500
	MaxExport         Limit = 10000
	WindowSize        Limit = 500
	PipelineCacheSize Limit = 64
	SummarySample     Limit = 1000
	GlobalParallelism Limit = 8
	ProduceAttempts   Limit = 3
	MaxJobSize        Limit = 1000000
	PreviewBatch      Limit = 200
)

// Budget holds the time bounds of the feature.
type Budget time.Duration

const (
	MaxScanTime     Budget = Budget(10 * time.Second)
	LockTTL         Budget = Budget(60 * time.Second)
	SummaryTTL      Budget = Budget(15 * time.Second)
	IndexReadyWait  Budget = Budget(5 * time.Second)
	IdlePoll        Budget = Budget(2 * time.Second)
	ProduceBackoff  Budget = Budget(200 * time.Millisecond)
	TailReconnect   Budget = Budget(2 * time.Second)
	StudioCallLimit Budget = Budget(15 * time.Second)
)

// Duration returns the budget as a time.Duration.
func (b Budget) Duration() time.Duration { return time.Duration(b) }

// StateTopic describes the compacted topic holding resolved state (one per DLQ connection).
type StateTopic string

const StateTopicName StateTopic = "router.dlq.state"

// StateTopicSpec holds the creation parameters of the state topic.
type StateTopicSpec int

const (
	StateTopicPartitions  StateTopicSpec = 6
	StateTopicReplication StateTopicSpec = -1
)

// TopicConfig names broker topic configs the feature sets.
type TopicConfig string

const (
	ConfigCleanupPolicy TopicConfig = "cleanup.policy"
	CleanupCompact      TopicConfig = "compact"
)
