package loadgen

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/test/stress/kit/pacing"
	"github.com/eventrouter/router/test/stress/kit/payload"
	"github.com/eventrouter/router/test/stress/kit/stream"
)

// ProduceConfig is everything `loadgen produce` needs.
type ProduceConfig struct {
	Brokers      []string
	Topics       []string
	Partitions   int32
	Plan         pacing.Plan
	Lanes        int
	Streams      int
	StreamPrefix string
	Keys         payload.KeyMode
	Template     []byte
	Size         int
	BadRatio     float64
	Acks         AcksMode
	Linger       time.Duration
	BatchBytes   int32
	Compression  Codec
	MaxBuffered  int
	Interval     time.Duration
	FlushTimeout time.Duration
}

// Producer runs a load plan against one topic.
type Producer struct {
	cfg      ProduceConfig
	client   *kgo.Client
	lanes    []*lane
	counters *counters
	out      *stream.Writer
	runCtx   context.Context
}

// counters are the producer's running totals, shared by lanes and ack callbacks.
type counters struct {
	sent, acked, errors atomic.Uint64
	step                atomic.Int64
	firstErr            atomic.Value
	perStep             []stepCounters
}

// stepCounters attribute acks to the step their record was sent in.
type stepCounters struct {
	sent, acked, errors atomic.Uint64
}

// NewProducer connects and prepares one lane per configured goroutine.
func NewProducer(cfg ProduceConfig, out *stream.Writer) (*Producer, error) {
	opts, err := ProducerOptions{}.Build(cfg)
	if err != nil {
		return nil, err
	}
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	tpl, err := payload.Compile(cfg.Template, cfg.Size)
	if err != nil {
		client.Close()
		return nil, err
	}
	p := &Producer{cfg: cfg, client: client, out: out, counters: &counters{perStep: make([]stepCounters, len(cfg.Plan.Steps))}}
	streams := payload.Streams{Prefix: cfg.StreamPrefix, Total: cfg.Streams, Lanes: cfg.Lanes}
	for i := 0; i < cfg.Lanes; i++ {
		l, err := newLane(i, streams.Owned(i), tpl, cfg, p)
		if err != nil {
			client.Close()
			return nil, err
		}
		p.lanes = append(p.lanes, l)
	}
	return p, nil
}

// Run executes every step of the plan (or until ctx ends), flushes and returns the summary.
func (p *Producer) Run(ctx context.Context) stream.ProduceSummary {
	defer p.client.Close()
	p.runCtx = ctx
	started := time.Now()
	stopReport := p.startReporter(ctx)
	var steps []stream.StepReport
	for i, step := range p.cfg.Plan.Steps {
		if ctx.Err() != nil {
			break
		}
		steps = append(steps, p.runStep(ctx, i, step))
	}
	flushCtx, cancel := context.WithTimeout(context.Background(), p.cfg.FlushTimeout)
	defer cancel()
	_ = p.client.Flush(flushCtx)
	stopReport()
	return p.summary(steps, time.Since(started))
}

func (p *Producer) runStep(ctx context.Context, index int, step pacing.Step) stream.StepReport {
	p.counters.step.Store(int64(index))
	begin := time.Now()
	report := stream.StepReport{Index: index, TargetRate: step.Rate, StartNs: begin.UnixNano()}
	p.out.Emit(stream.Line{Kind: stream.KindStep, Role: stream.RoleProduce, AtNs: report.StartNs, Step: &report})
	stepCtx, cancel := context.WithDeadline(ctx, begin.Add(step.Duration))
	defer cancel()
	var wg sync.WaitGroup
	for _, l := range p.lanes {
		wg.Add(1)
		go func(l *lane) {
			defer wg.Done()
			l.run(stepCtx, index, step.Rate/float64(len(p.lanes)))
		}(l)
	}
	wg.Wait()
	end := time.Now()
	sc := &p.counters.perStep[index]
	report.EndNs, report.DurationSec = end.UnixNano(), end.Sub(begin).Seconds()
	report.Sent, report.Acked, report.Errors = sc.sent.Load(), sc.acked.Load(), sc.errors.Load()
	report.AchievedRate = float64(report.Acked) / report.DurationSec
	return report
}

func (p *Producer) onAck(step int) func(*kgo.Record, error) {
	sc := &p.counters.perStep[step]
	return func(_ *kgo.Record, err error) {
		if err != nil {
			sc.errors.Add(1)
			p.counters.errors.Add(1)
			if !errors.Is(err, context.Canceled) {
				p.counters.firstErr.CompareAndSwap(nil, err.Error())
			}
			return
		}
		sc.acked.Add(1)
		p.counters.acked.Add(1)
	}
}

func (p *Producer) countSent(step int, n uint64) {
	p.counters.perStep[step].sent.Add(n)
	p.counters.sent.Add(n)
}

func (p *Producer) summary(steps []stream.StepReport, took time.Duration) stream.ProduceSummary {
	s := stream.ProduceSummary{Steps: steps, Sent: p.counters.sent.Load(), Acked: p.counters.acked.Load(), Errors: p.counters.errors.Load(), DurationSec: took.Seconds()}
	if s.DurationSec > 0 {
		s.AchievedRate = float64(s.Acked) / s.DurationSec
	}
	if e, ok := p.counters.firstErr.Load().(string); ok {
		s.FirstError = e
	}
	return s
}

func (p *Producer) startReporter(ctx context.Context) func() {
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		tick := time.NewTicker(p.cfg.Interval)
		defer tick.Stop()
		var lastSent, lastAcked, lastErr uint64
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
				lastSent, lastAcked, lastErr = p.emitWindow(lastSent, lastAcked, lastErr)
			}
		}
	}()
	return func() { close(done); <-finished }
}

func (p *Producer) emitWindow(lastSent, lastAcked, lastErr uint64) (uint64, uint64, uint64) {
	sent, acked, errs := p.counters.sent.Load(), p.counters.acked.Load(), p.counters.errors.Load()
	sec := p.cfg.Interval.Seconds()
	w := stream.ProduceWindow{
		Step: int(p.counters.step.Load()), SentRate: float64(sent-lastSent) / sec, AckedRate: float64(acked-lastAcked) / sec,
		Errors: errs - lastErr, Buffered: p.client.BufferedProduceRecords(),
	}
	p.out.Emit(stream.Line{Kind: stream.KindWindow, Role: stream.RoleProduce, AtNs: time.Now().UnixNano(), Produce: &w})
	return sent, acked, errs
}

// ProducerOptions assembles the franz-go client options of a ProduceConfig.
type ProducerOptions struct{}

// Build returns the client options.
func (ProducerOptions) Build(cfg ProduceConfig) ([]kgo.Opt, error) {
	acks, err := ClientOptions{}.Acks(cfg.Acks)
	if err != nil {
		return nil, err
	}
	comp, err := ClientOptions{}.Compression(cfg.Compression)
	if err != nil {
		return nil, err
	}
	opts := append([]kgo.Opt{kgo.SeedBrokers(cfg.Brokers...), kgo.ProducerLinger(cfg.Linger), comp}, acks...)
	if cfg.BatchBytes > 0 {
		opts = append(opts, kgo.ProducerBatchMaxBytes(cfg.BatchBytes))
	}
	if cfg.MaxBuffered > 0 {
		opts = append(opts, kgo.MaxBufferedRecords(cfg.MaxBuffered))
	}
	return opts, nil
}
