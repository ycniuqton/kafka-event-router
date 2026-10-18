package preview

import (
	"context"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/kafka"
)

// DemoSampleSettings shape the synthetic demo samples: generator seed, partition spread and event spacing
// (the same numbers as the connections demo cluster, so samples look like what the demo feed produces).
type DemoSampleSettings struct {
	Seed       uint64
	Partitions int32
	Interval   time.Duration
}

// DefaultDemoSampleSettings mirrors kafka.DefaultDemoSettings.
func DefaultDemoSampleSettings() DemoSampleSettings {
	d := kafka.DefaultDemoSettings()
	return DemoSampleSettings{Seed: d.Seed, Partitions: d.Partitions, Interval: d.Interval}
}

// DemoSource serves synthetic `order-events` samples for the demo workspace (FR-9): no Kafka is read.
// Event i is the same in every process (connections' DemoOrderGenerator); only timestamps follow the clock.
type DemoSource struct {
	Settings DemoSampleSettings
	Clock    func() time.Time
}

// NewDemoSource returns the source with the default demo settings.
func NewDemoSource(clock func() time.Time) DemoSource {
	return DemoSource{Settings: DefaultDemoSampleSettings(), Clock: clock}
}

func (DemoSource) Mode() SampleMode { return ModeDemo }

// Load generates the latest `count` demo order events, oldest first, within the budget.
func (s DemoSource) Load(_ context.Context, req LoadRequest, budget *kafka.SamplingBudget) (LoadResult, error) {
	if err := (DemoTopics{}).Check(req.Topics); err != nil {
		return LoadResult{}, err
	}
	count := req.EffectiveCount()
	now := s.Clock()
	gen := kafka.NewDemoOrderGenerator(s.Settings.Seed, 0)
	res := LoadResult{Records: make([]SampleRecord, 0, count)}
	for i := range count {
		at := now.Add(-time.Duration(count-1-i) * s.Settings.Interval)
		rec := s.record(i, gen.Next(at))
		if !budget.Admit(rec.Size(), s.Clock()) {
			break
		}
		res.Records = append(res.Records, rec)
		res.Bytes += rec.Size()
	}
	res.StopReason = budget.Reason()
	return res, nil
}

// record places generated event i on its demo partition and offset.
func (s DemoSource) record(i int, r *kgo.Record) SampleRecord {
	parts := max(s.Settings.Partitions, 1)
	rec := SampleRecord{
		Seq: i, Topic: r.Topic, Partition: int32(i) % parts, Offset: int64(i) / int64(parts),
		Timestamp: r.Timestamp.UTC(), Key: r.Key, Value: r.Value, Headers: make([]engine.Header, 0, len(r.Headers)),
	}
	for _, h := range r.Headers {
		rec.Headers = append(rec.Headers, engine.Header{Key: h.Key, Value: h.Value})
	}
	return rec
}

// DemoTopics validates the requested topics: the demo source only serves `order-events`.
type DemoTopics struct{}

// Check accepts no topic or the demo topic; any other topic is reported at its index.
func (DemoTopics) Check(topics []string) error {
	for i, t := range topics {
		if t != string(kafka.DemoOrderEvents) {
			return ErrTopicNotFound.At(FieldTopics.Indexed(i))
		}
	}
	return nil
}
