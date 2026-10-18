package checks

import (
	"context"
	"errors"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/pkg/kafka"
)

// ReadPolicy bounds the single-record read: how long to wait and how many bytes one record may use.
type ReadPolicy int64

const (
	ReadWait     ReadPolicy = ReadPolicy(3 * time.Second)
	ReadCount    ReadPolicy = 1
	ReadMaxBytes ReadPolicy = 10 << 20
)

// RecordReader reads one record of a topic without a consumer group or commits.
type RecordReader interface {
	ReadOne(ctx context.Context, client kafka.Client, topic kafka.TopicInfo) error
}

// SamplerRecordReader reads the newest record through live-preview's PartitionSampler
// (manual assignment, no group, no commits); an empty or quiet topic passes.
type SamplerRecordReader struct{}

// ReadOne returns the first marks or fetch error; the client stays open for the next checks.
func (SamplerRecordReader) ReadOne(ctx context.Context, client kafka.Client, topic kafka.TopicInfo) error {
	raw := client.Raw()
	marks, err := kafka.NewFranzMarksReader(raw).Marks(ctx, []string{topic.Name}, time.Time{})
	if err != nil {
		return err
	}
	windows := kafka.LatestOffsetPlanner{}.Plan(kafka.SamplingPlan{Count: int(ReadCount)}, marks)
	budget := kafka.NewSamplingBudget(int(ReadCount), int64(ReadMaxBytes), time.Now().Add(time.Duration(ReadWait)))
	_, _, err = kafka.NewFranzPartitionSampler(raw).Read(ctx, windows, budget)
	return err
}

// ReadCheck reads one record of the chosen sample topic; without a topic the step is skipped.
type ReadCheck struct {
	Reader RecordReader
}

// Step implements connections.TestCheck.
func (ReadCheck) Step() connections.TestStep { return connections.StepCanRead }

// Run implements connections.TestCheck.
func (c ReadCheck) Run(ctx context.Context, s *connections.TestSession) connections.StepOutcome {
	if s.Options.ReadTopic == "" {
		return connections.Skipped()
	}
	topic, outcome := TopicLookup{}.Find(ctx, s, s.Options.ReadTopic)
	if outcome != nil {
		return *outcome
	}
	if err := c.Reader.ReadOne(ctx, s.Client, topic); err != nil {
		return connections.Failed(err)
	}
	return connections.Passed()
}

// WriteCheck produces one marked record to an existing test topic; without a topic the step is skipped.
type WriteCheck struct{}

// Step implements connections.TestCheck.
func (WriteCheck) Step() connections.TestStep { return connections.StepCanWrite }

// Run implements connections.TestCheck.
func (WriteCheck) Run(ctx context.Context, s *connections.TestSession) connections.StepOutcome {
	if s.Options.WriteTopic == "" {
		return connections.Skipped()
	}
	if _, outcome := (TopicLookup{}).Find(ctx, s, s.Options.WriteTopic); outcome != nil {
		return *outcome
	}
	record := &kgo.Record{
		Topic:   s.Options.WriteTopic,
		Value:   []byte(connections.TestRecordValue),
		Headers: []kgo.RecordHeader{{Key: string(connections.HeaderTestRecord), Value: []byte(connections.TestRecordValue)}},
	}
	if err := s.Client.Raw().ProduceSync(ctx, record).FirstErr(); err != nil {
		return connections.Failed(err)
	}
	return connections.Passed()
}

// ErrTopicMissing is returned when a chosen test topic does not exist.
var ErrTopicMissing = errors.New("connections: test topic not found")

// TopicLookup resolves a chosen test topic (existing topics only).
type TopicLookup struct{}

// Find returns the topic, or the failed outcome to report.
func (TopicLookup) Find(ctx context.Context, s *connections.TestSession, name string) (kafka.TopicInfo, *connections.StepOutcome) {
	topics, err := s.Client.Admin().DescribeTopics(ctx, name)
	if err != nil {
		out := connections.Failed(err)
		return kafka.TopicInfo{}, &out
	}
	if len(topics) == 0 {
		out := connections.FailedAs(kafka.ReasonTopicNotFound, ErrTopicMissing)
		return kafka.TopicInfo{}, &out
	}
	return topics[0], nil
}
