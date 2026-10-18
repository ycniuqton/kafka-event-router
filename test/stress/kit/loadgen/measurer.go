package loadgen

import (
	"context"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/test/stress/kit/kafkaops"
	"github.com/eventrouter/router/test/stress/kit/latency"
	"github.com/eventrouter/router/test/stress/kit/seqtrack"
	"github.com/eventrouter/router/test/stress/kit/stream"
)

// MeasureConfig is everything `loadgen measure` needs.
type MeasureConfig struct {
	Brokers       []string
	Topics        []string
	ReadCommitted bool
	From          kafkaops.StartPoint
	SeqSource     SeqSource
	TrackBy       TrackBy
	Expect        uint64
	MaxDuration   time.Duration
	DrainIdle     time.Duration
	DrainMax      time.Duration
	Interval      time.Duration
	PollTimeout   time.Duration
}

// MeasureTuning holds the consumer settings that keep the measurer's own delay small.
var MeasureTuning = struct {
	FetchMaxWait time.Duration
}{FetchMaxWait: 100 * time.Millisecond}

// Measurer consumes output topics without a consumer group and accounts every record.
type Measurer struct {
	cfg      MeasureConfig
	client   *kgo.Client
	extract  Extractor
	trackKey func(topic, stream string) string
	out      *stream.Writer
	total    *latency.Histogram
	window   *latency.Histogram
	tracker  *seqtrack.Tracker
	byTopic  map[string]uint64
	winTopic map[string]uint64
	unparsed uint64
	lastSeen time.Time
	stopAt   time.Time
	started  time.Time
	nextEmit time.Time
}

// NewMeasurer resolves exact start offsets (so "ready" means nothing produced later can be missed) and connects.
func NewMeasurer(ctx context.Context, cfg MeasureConfig, out *stream.Writer) (*Measurer, error) {
	extract, err := ExtractorManager{}.For(cfg.SeqSource)
	if err != nil {
		return nil, err
	}
	if cfg.TrackBy == "" {
		cfg.TrackBy = TrackStream
	}
	trackKey, err := ExtractorManager{}.TrackKey(cfg.TrackBy)
	if err != nil {
		return nil, err
	}
	adm, err := kafkaops.Dial(cfg.Brokers)
	if err != nil {
		return nil, err
	}
	offsets, err := adm.Offsets(ctx, cfg.From, cfg.ReadCommitted, cfg.Topics...)
	adm.Close()
	if err != nil {
		return nil, err
	}
	opts := []kgo.Opt{kgo.SeedBrokers(cfg.Brokers...), kgo.ConsumePartitions(offsets), kgo.FetchMaxWait(MeasureTuning.FetchMaxWait)}
	if cfg.ReadCommitted {
		opts = append(opts, kgo.FetchIsolationLevel(kgo.ReadCommitted()))
	}
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return &Measurer{
		cfg: cfg, client: client, extract: extract, trackKey: trackKey, out: out, total: latency.New(), window: latency.New(),
		tracker: seqtrack.New(), byTopic: map[string]uint64{}, winTopic: map[string]uint64{},
	}, nil
}

// Run consumes until ctx ends (then drains until DrainIdle passes without records), Expect unique events arrived,
// or MaxDuration elapsed; returns the summary.
func (m *Measurer) Run(ctx context.Context) stream.MeasureSummary {
	defer m.client.Close()
	m.started = time.Now()
	m.lastSeen, m.nextEmit = m.started, m.started.Add(m.cfg.Interval)
	m.out.Emit(stream.Line{Kind: stream.KindReady, Role: stream.RoleMeasure, AtNs: m.started.UnixNano()})
	for !m.finished(ctx) {
		m.poll()
		m.maybeEmit(time.Now())
	}
	m.emitWindow(time.Now())
	return m.summary()
}

func (m *Measurer) finished(ctx context.Context) bool {
	now := time.Now()
	if m.cfg.MaxDuration > 0 && now.Sub(m.started) >= m.cfg.MaxDuration {
		return true
	}
	if m.cfg.Expect > 0 && m.tracker.Unique() >= m.cfg.Expect {
		return true
	}
	if ctx.Err() == nil {
		return false
	}
	if m.stopAt.IsZero() {
		m.stopAt = now
	}
	return now.Sub(m.lastSeen) >= m.cfg.DrainIdle || (m.cfg.DrainMax > 0 && now.Sub(m.stopAt) >= m.cfg.DrainMax)
}

func (m *Measurer) poll() {
	pctx, cancel := context.WithTimeout(context.Background(), m.cfg.PollTimeout)
	defer cancel()
	fetches := m.client.PollFetches(pctx)
	now := time.Now()
	fetches.EachRecord(func(r *kgo.Record) { m.account(r, now) })
}

func (m *Measurer) account(r *kgo.Record, now time.Time) {
	m.lastSeen = now
	m.byTopic[r.Topic]++
	m.winTopic[r.Topic]++
	var st Stamp
	sent, ok := ExtractorManager{}.SentTime(r)
	m.extract.Identity(r, &st)
	if !ok || !st.HasSeq {
		m.unparsed++
		return
	}
	d := now.Sub(time.Unix(0, sent))
	m.total.Record(d)
	m.window.Record(d)
	m.tracker.Observe(m.trackKey(r.Topic, st.Stream), st.Seq)
}

func (m *Measurer) maybeEmit(now time.Time) {
	if now.Before(m.nextEmit) {
		return
	}
	m.emitWindow(now)
	m.nextEmit = m.nextEmit.Add(m.cfg.Interval)
	if m.nextEmit.Before(now) {
		m.nextEmit = now.Add(m.cfg.Interval)
	}
}

func (m *Measurer) emitWindow(now time.Time) {
	w := stream.MeasureWindow{
		Count: m.window.Count(), Rate: float64(m.window.Count()) / m.cfg.Interval.Seconds(),
		Latency: m.window.Summary(), Histogram: m.window.Sparse(), ByTopic: m.winTopic,
	}
	m.out.Emit(stream.Line{Kind: stream.KindWindow, Role: stream.RoleMeasure, AtNs: now.UnixNano(), Measure: &w})
	m.window = latency.New()
	m.winTopic = map[string]uint64{}
}

func (m *Measurer) summary() stream.MeasureSummary {
	return stream.MeasureSummary{
		Latency: m.total.Summary(), Delivery: m.tracker.Summary(m.cfg.Expect), ByTopic: m.byTopic,
		DurationSec: time.Since(m.started).Seconds(), Unparsed: m.unparsed,
	}
}
