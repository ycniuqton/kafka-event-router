package loadgen

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/test/stress/kit/kafkaops"
	"github.com/eventrouter/router/test/stress/kit/pacing"
	"github.com/eventrouter/router/test/stress/kit/payload"
	"github.com/eventrouter/router/test/stress/kit/stream"
)

func TestProduceThenMeasureIsComplete(t *testing.T) {
	topics := []string{"stress-t-a", "stress-t-b"}
	cluster, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(3, topics...))
	if err != nil {
		t.Fatal(err)
	}
	defer cluster.Close()
	brokers := cluster.ListenAddrs()
	var out bytes.Buffer
	w := stream.NewWriter(&out)
	ctx := context.Background()
	m, err := NewMeasurer(ctx, MeasureConfig{Brokers: brokers, Topics: topics, From: kafkaops.FromEnd, SeqSource: SeqFromHeader, MaxDuration: 20 * time.Second, DrainIdle: time.Second, Interval: time.Second, PollTimeout: 200 * time.Millisecond}, w)
	if err != nil {
		t.Fatal(err)
	}
	cfg := ProduceConfig{
		Brokers: brokers, Topics: topics, Plan: pacing.Plan{Steps: []pacing.Step{{Rate: 2000, Duration: 500 * time.Millisecond}, {Rate: 2000, Duration: 500 * time.Millisecond}}},
		Lanes: 3, Streams: 8, StreamPrefix: "s", Keys: payload.KeyRoundRobin, Template: []byte(payload.DefaultTemplate), Size: 512,
		Acks: AcksAll, Compression: CodecNone, Interval: 200 * time.Millisecond, FlushTimeout: 10 * time.Second,
	}
	p, err := NewProducer(cfg, w)
	if err != nil {
		t.Fatal(err)
	}
	mctx, stop := context.WithCancel(ctx)
	done := make(chan stream.MeasureSummary)
	go func() { done <- m.Run(mctx) }()
	ps := p.Run(ctx)
	stop()
	ms := <-done
	if ps.Acked < 1980 || ps.Acked != ps.Sent || ps.Errors != 0 || len(ps.Steps) != 2 || ps.Steps[1].Acked < 990 {
		t.Fatalf("produce summary = %+v", ps)
	}
	d := ms.Delivery
	if d.Unique != ps.Acked || d.Duplicates != 0 || d.Gaps != 0 || d.Streams != 8 || ms.Unparsed != 0 {
		t.Errorf("delivery = %+v", d)
	}
	if ms.ByTopic[topics[0]] == 0 || ms.ByTopic[topics[1]] == 0 || ms.Latency.Count != ps.Acked || ms.Latency.P99Ms <= 0 {
		t.Errorf("measure = %+v", ms)
	}
}

func TestPayloadExtractorReadsSeq(t *testing.T) {
	tpl, _ := payload.Compile([]byte(payload.DefaultTemplate), 0)
	rec := recordWith(tpl.Append(nil, payload.Event{Seq: 77, Stream: "s-3"}))
	var st Stamp
	extractors[SeqFromPayload].Identity(rec, &st)
	if !st.HasSeq || st.Seq != 77 || st.Stream != "s-3" {
		t.Errorf("stamp = %+v", st)
	}
	if _, err := (ExtractorManager{}).For("nope"); err == nil {
		t.Error("unknown source accepted")
	}
}

func recordWith(value []byte) *kgo.Record {
	return &kgo.Record{Value: value}
}
