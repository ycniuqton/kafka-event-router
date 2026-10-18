// Package connections is the saved-Kafka-cluster domain: CRUD with write-only secrets, the live test
// checklist, the background health probe, the topic browser and topic creation.
package connections

import "time"

// EnvTag is the environment a cluster belongs to.
type EnvTag string

const (
	EnvDev     EnvTag = "dev"
	EnvStaging EnvTag = "staging"
	EnvProd    EnvTag = "prod"
)

// EnvTags lists every valid tag.
var EnvTags = []EnvTag{EnvDev, EnvStaging, EnvProd}

// Valid reports whether e is a known tag.
func (e EnvTag) Valid() bool {
	for _, t := range EnvTags {
		if t == e {
			return true
		}
	}
	return false
}

// HealthStatus is the health of a connection after its last check.
type HealthStatus string

const (
	HealthUnknown     HealthStatus = "unknown"
	HealthHealthy     HealthStatus = "healthy"
	HealthDegraded    HealthStatus = "degraded"
	HealthUnreachable HealthStatus = "unreachable"
	HealthAuthFailed  HealthStatus = "auth_failed"
)

// HealthStatuses lists every valid status.
var HealthStatuses = []HealthStatus{HealthUnknown, HealthHealthy, HealthDegraded, HealthUnreachable, HealthAuthFailed}

// Valid reports whether h is a known status.
func (h HealthStatus) Valid() bool {
	for _, s := range HealthStatuses {
		if s == h {
			return true
		}
	}
	return false
}

// TestStep is one check of the connection test checklist.
type TestStep string

const (
	StepBrokersReachable TestStep = "brokers_reachable"
	StepAuthenticated    TestStep = "authenticated"
	StepListTopics       TestStep = "list_topics"
	StepCanRead          TestStep = "can_read"
	StepCanWrite         TestStep = "can_write"
	StepSchemaRegistry   TestStep = "schema_registry"
	StepTopicAdmin       TestStep = "topic_admin"
)

// StepState is the state of one step in a test run.
type StepState string

const (
	StatePending StepState = "pending"
	StateRunning StepState = "running"
	StatePassed  StepState = "passed"
	StateFailed  StepState = "failed"
	StateSkipped StepState = "skipped"
)

// TestLimits bounds a test run (NFR-2).
type TestLimits time.Duration

const (
	StepTimeout TestLimits = TestLimits(10 * time.Second)
	RunTimeout  TestLimits = TestLimits(30 * time.Second)
)

// TopicDefaults are applied to create-topic requests that omit a value.
type TopicDefaults int64

const (
	DefaultTopicPartitions TopicDefaults = 6
	DefaultReplication     TopicDefaults = -1
	DefaultRetentionMs     TopicDefaults = TopicDefaults(7 * 24 * time.Hour / time.Millisecond)
	MaxTopicPartitions     TopicDefaults = 1000
	MaxTopicNameLength     TopicDefaults = 249
	MaxReplicationFactor   TopicDefaults = 32
)

// ConnectionLimits bounds connection fields.
type ConnectionLimits int

const (
	MaxNameLength        ConnectionLimits = 100
	MaxDescriptionLength ConnectionLimits = 1000
	MaxBootstrapServers  ConnectionLimits = 50
	MaxPort              ConnectionLimits = 65535
)

// TestCheckMarker identifies records written by the "can write" check.
type TestCheckMarker string

const (
	HeaderTestRecord TestCheckMarker = "x-router-test"
	TestRecordValue  TestCheckMarker = "connection-test"
)

// InternalTopic recognises topics hidden from the browser by default.
type InternalTopic string

const (
	InternalPrefix  InternalTopic = "__"
	SchemasTopic    InternalTopic = "_schemas"
	RouterDLQPrefix InternalTopic = "router.dlq."
)
