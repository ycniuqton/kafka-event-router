package preview

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/kafka"
)

// SamplingClient is a group-less Kafka client opened for one connection.
type SamplingClient interface {
	kafka.PartitionMarksReader
	kafka.PartitionSampler
	Close()
}

// SamplingClientFactory opens sampling clients for saved connections (secrets resolved inside).
type SamplingClientFactory interface {
	Kind() SamplerKind
	Open(ctx context.Context, connectionID string) (SamplingClient, error)
}

// SamplingClientRegistry selects the configured factory (ROUTER_PREVIEW_KAFKA_SAMPLER).
type SamplingClientRegistry struct {
	factories map[SamplerKind]SamplingClientFactory
}

// NewSamplingClientRegistry registers the unavailable fallback plus the given factories.
func NewSamplingClientRegistry(fs ...SamplingClientFactory) *SamplingClientRegistry {
	r := &SamplingClientRegistry{factories: map[SamplerKind]SamplingClientFactory{}}
	r.Register(UnavailableSamplingFactory{})
	for _, f := range fs {
		r.Register(f)
	}
	return r
}

// Register adds or replaces a factory.
func (r *SamplingClientRegistry) Register(f SamplingClientFactory) {
	r.factories[f.Kind()] = f
}

// Build returns the factory of kind.
func (r *SamplingClientRegistry) Build(kind SamplerKind) (SamplingClientFactory, error) {
	f, ok := r.factories[kind]
	if !ok {
		return nil, fmt.Errorf("preview: no kafka sampler registered for %q", kind)
	}
	return f, nil
}

// UnavailableSamplingFactory answers every open with source-unavailable until the franz
// sampler (pkg/kafka, needs connections' client factory) is registered.
type UnavailableSamplingFactory struct{}

func (UnavailableSamplingFactory) Kind() SamplerKind { return SamplerUnavailable }

func (UnavailableSamplingFactory) Open(context.Context, string) (SamplingClient, error) {
	return nil, ErrSamplerMissing
}

// KafkaSampling runs the shared Kafka load steps: open, marks, missing-topic check, plan, read, convert.
type KafkaSampling struct {
	Clients SamplingClientFactory
}

// Load samples the topics of req with planner and returns records ordered by timestamp.
func (k KafkaSampling) Load(ctx context.Context, req LoadRequest, from time.Time, plan kafka.SamplingPlan, planner kafka.OffsetPlanner, budget *kafka.SamplingBudget) (LoadResult, error) {
	if err := (LoadRequestCheck{}).Kafka(req); err != nil {
		return LoadResult{}, err
	}
	client, err := k.Clients.Open(ctx, req.ConnectionID)
	if err != nil {
		return LoadResult{}, err
	}
	defer client.Close()
	marks, err := client.Marks(ctx, req.Topics, from)
	if err != nil {
		return LoadResult{}, ErrSourceUnavailable.Wrap(err)
	}
	if err := (MissingTopics{}).Check(req.Topics, marks); err != nil {
		return LoadResult{}, err
	}
	raw, reason, err := client.Read(ctx, planner.Plan(plan, marks), budget)
	if err != nil {
		return LoadResult{}, ErrSourceUnavailable.Wrap(err)
	}
	return RecordConverter{}.Result(RecordConverter{}.ByTimestamp(raw), reason), nil
}

// LoadRequestCheck validates request members shared by sources.
type LoadRequestCheck struct{}

// Kafka requires a connection and at least one topic.
func (LoadRequestCheck) Kafka(req LoadRequest) error {
	if req.ConnectionID == "" {
		return ErrConnectionNeeded.At(string(FieldConnectionID))
	}
	if len(req.Topics) == 0 {
		return ErrTopicsRequired.At(string(FieldTopics))
	}
	return nil
}

// MissingTopics reports the first requested topic without partitions.
type MissingTopics struct{}

// Check returns a 404 problem pointing at /topics/{i} for an unknown topic.
func (MissingTopics) Check(topics []string, marks []kafka.PartitionMarks) error {
	known := map[string]bool{}
	for _, m := range marks {
		known[m.Topic] = true
	}
	for i, t := range topics {
		if !known[t] {
			return ErrTopicNotFound.At(FieldTopics.Indexed(i))
		}
	}
	return nil
}

// RecordConverter turns sampled Kafka records into session records.
type RecordConverter struct{}

// ByTimestamp orders records by timestamp, then topic/partition/offset.
func (RecordConverter) ByTimestamp(recs []kafka.SampledRecord) []kafka.SampledRecord {
	sort.SliceStable(recs, func(i, j int) bool {
		a, b := recs[i], recs[j]
		if !a.Timestamp.Equal(b.Timestamp) {
			return a.Timestamp.Before(b.Timestamp)
		}
		if a.Topic != b.Topic || a.Partition != b.Partition {
			return a.Topic < b.Topic || (a.Topic == b.Topic && a.Partition < b.Partition)
		}
		return a.Offset < b.Offset
	})
	return recs
}

// Result numbers the records and sums their size.
func (RecordConverter) Result(recs []kafka.SampledRecord, reason kafka.StopReason) LoadResult {
	out := LoadResult{Records: make([]SampleRecord, 0, len(recs)), StopReason: reason}
	for i, r := range recs {
		rec := SampleRecord{Seq: i, Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Timestamp: r.Timestamp, Key: r.Key, Value: r.Value, Headers: []engine.Header{}}
		for _, h := range r.Headers {
			rec.Headers = append(rec.Headers, engine.Header{Key: h.Key, Value: h.Value})
		}
		out.Bytes += rec.Size()
		out.Records = append(out.Records, rec)
	}
	return out
}

// KafkaLatestSource loads the newest N records across the topics' partitions.
type KafkaLatestSource struct {
	Sampling KafkaSampling
	Planner  kafka.OffsetPlanner
}

func (KafkaLatestSource) Mode() SampleMode { return ModeLatest }

// Load plans per-partition windows, reads them and keeps the latest N by timestamp.
func (s KafkaLatestSource) Load(ctx context.Context, req LoadRequest, budget *kafka.SamplingBudget) (LoadResult, error) {
	count := req.EffectiveCount()
	res, err := s.Sampling.Load(ctx, req, time.Time{}, kafka.SamplingPlan{Count: count}, s.Planner, budget)
	if err != nil || len(res.Records) <= count {
		return res, err
	}
	kept := res.Records[len(res.Records)-count:]
	return RecordConverter{}.Renumber(kept, res.StopReason), nil
}

// Renumber re-sequences kept records and recomputes their size.
func (RecordConverter) Renumber(recs []SampleRecord, reason kafka.StopReason) LoadResult {
	out := LoadResult{Records: make([]SampleRecord, 0, len(recs)), StopReason: reason}
	for i, r := range recs {
		r.Seq = i
		out.Bytes += r.Size()
		out.Records = append(out.Records, r)
	}
	return out
}

// KafkaTimeRangeSource loads records whose timestamps fall in [from, to].
type KafkaTimeRangeSource struct {
	Sampling KafkaSampling
	Planner  kafka.OffsetPlanner
}

func (KafkaTimeRangeSource) Mode() SampleMode { return ModeTimeRange }

// Load resolves offsets by timestamp and reads up to the HWM snapshot or past `to`.
func (s KafkaTimeRangeSource) Load(ctx context.Context, req LoadRequest, budget *kafka.SamplingBudget) (LoadResult, error) {
	if req.From == nil || req.To == nil || !req.From.Before(*req.To) {
		return LoadResult{}, ErrTimeRange.At(string(FieldFrom))
	}
	plan := kafka.SamplingPlan{Count: int(MaxEvents), From: *req.From, To: *req.To}
	return s.Sampling.Load(ctx, req, *req.From, plan, s.Planner, budget)
}
