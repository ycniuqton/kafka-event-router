package preview_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/pkg/kafka"
)

// fakeKafka is an in-memory topic used through the SamplingClient port.
type fakeKafka struct {
	marks   []kafka.PartitionMarks
	records []kafka.SampledRecord
	read    []kafka.PartitionWindow
}

func (f *fakeKafka) Kind() preview.SamplerKind { return preview.SamplerFranz }
func (f *fakeKafka) Open(context.Context, string) (preview.SamplingClient, error) {
	return f, nil
}
func (f *fakeKafka) Close() {}
func (f *fakeKafka) Marks(_ context.Context, topics []string, _ time.Time) ([]kafka.PartitionMarks, error) {
	out := []kafka.PartitionMarks{}
	for _, m := range f.marks {
		for _, t := range topics {
			if m.Topic == t {
				out = append(out, m)
			}
		}
	}
	return out, nil
}
func (f *fakeKafka) Read(_ context.Context, ws []kafka.PartitionWindow, b *kafka.SamplingBudget) ([]kafka.SampledRecord, kafka.StopReason, error) {
	f.read = ws
	out := []kafka.SampledRecord{}
	for _, w := range ws {
		for _, r := range f.records {
			if r.Partition == w.Partition && r.Offset >= w.Start && r.Offset < w.End && b.Admit(r.Size(), time.Time{}) {
				out = append(out, r)
			}
		}
	}
	return out, b.Reason(), nil
}

func newFakeKafka() *fakeKafka {
	base := time.Unix(0, 0)
	f := &fakeKafka{marks: []kafka.PartitionMarks{
		{Topic: "orders", Partition: 0, Low: 0, High: 10, FromOffset: 5},
		{Topic: "orders", Partition: 1, Low: 0, High: 10, FromOffset: 5},
	}}
	for p := int32(0); p < 2; p++ {
		for o := int64(0); o < 10; o++ {
			f.records = append(f.records, kafka.SampledRecord{Topic: "orders", Partition: p, Offset: o, Timestamp: base.Add(time.Duration(o*2+int64(p)) * time.Second), Value: []byte(`{}`)})
		}
	}
	return f
}

func TestKafkaLatestSourceKeepsNewestByTimestamp(t *testing.T) {
	f := newFakeKafka()
	src := preview.KafkaLatestSource{Sampling: preview.KafkaSampling{Clients: f}, Planner: kafka.LatestOffsetPlanner{}}
	res, err := src.Load(context.Background(), preview.LoadRequest{ConnectionID: "c", Topics: []string{"orders"}, Count: 3}, newBudget())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Records) != 3 || res.StopReason != kafka.StopComplete {
		t.Fatalf("records = %d %s", len(res.Records), res.StopReason)
	}
	for i := 1; i < len(res.Records); i++ {
		if res.Records[i].Timestamp.Before(res.Records[i-1].Timestamp) || res.Records[i].Seq != i {
			t.Fatalf("not ordered/numbered: %+v", res.Records)
		}
	}
	if last := res.Records[2]; last.Partition != 1 || last.Offset != 9 {
		t.Errorf("newest record = p%d o%d", last.Partition, last.Offset)
	}
}

func TestKafkaSourcesValidateRequests(t *testing.T) {
	f := newFakeKafka()
	sampling := preview.KafkaSampling{Clients: f}
	latest := preview.KafkaLatestSource{Sampling: sampling, Planner: kafka.LatestOffsetPlanner{}}
	ranged := preview.KafkaTimeRangeSource{Sampling: sampling, Planner: kafka.TimeRangeOffsetPlanner{}}
	from, to := time.Unix(10, 0), time.Unix(5, 0)
	cases := []struct {
		name string
		src  preview.SampleSource
		req  preview.LoadRequest
		path string
		kind problem.Kind
	}{
		{name: "missing topic", src: latest, req: preview.LoadRequest{ConnectionID: "c", Topics: []string{"orders", "nope"}}, path: "/topics/1", kind: preview.KindTopicNotFound},
		{name: "no connection", src: latest, req: preview.LoadRequest{Topics: []string{"orders"}}, path: "/connectionId", kind: preview.KindInvalidRequest},
		{name: "bad range", src: ranged, req: preview.LoadRequest{ConnectionID: "c", Topics: []string{"orders"}, From: &from, To: &to}, path: "/from", kind: preview.KindInvalidRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.src.Load(context.Background(), tc.req, newBudget())
			var pe *problem.Error
			if !errors.As(err, &pe) || pe.Path != tc.path || pe.Kind != tc.kind {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestKafkaTimeRangeSourceStartsAtResolvedOffsets(t *testing.T) {
	f := newFakeKafka()
	src := preview.KafkaTimeRangeSource{Sampling: preview.KafkaSampling{Clients: f}, Planner: kafka.TimeRangeOffsetPlanner{}}
	from, to := time.Unix(10, 0), time.Unix(100, 0)
	res, err := src.Load(context.Background(), preview.LoadRequest{ConnectionID: "c", Topics: []string{"orders"}, From: &from, To: &to}, newBudget())
	if err != nil || len(res.Records) != 10 || f.read[0].Start != 5 || !f.read[0].Until.Equal(to) {
		t.Fatalf("records %d windows %+v err %v", len(res.Records), f.read, err)
	}
}

func TestUnavailableSamplerIs502(t *testing.T) {
	clients, _ := preview.NewSamplingClientRegistry().Build(preview.SamplerUnavailable)
	src := preview.KafkaLatestSource{Sampling: preview.KafkaSampling{Clients: clients}, Planner: kafka.LatestOffsetPlanner{}}
	_, err := src.Load(context.Background(), preview.LoadRequest{ConnectionID: "c", Topics: []string{"t"}}, newBudget())
	if preview.NewProblemCatalog().Resolve(err).Status != 502 {
		t.Fatalf("err = %v", err)
	}
}
