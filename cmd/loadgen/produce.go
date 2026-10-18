package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/eventrouter/router/test/stress/kit/kafkaops"
	"github.com/eventrouter/router/test/stress/kit/loadgen"
	"github.com/eventrouter/router/test/stress/kit/pacing"
	"github.com/eventrouter/router/test/stress/kit/payload"
	"github.com/eventrouter/router/test/stress/kit/stream"
)

// Switch is a loadgen flag read before flag parsing.
type Switch string

const SwitchStopOnEOF Switch = "--stop-on-eof"

// ProduceDefaults are the defaults of `loadgen produce`.
var ProduceDefaults = struct {
	Brokers, Prefix string
	Lanes, Streams  int
	Size            int
	Linger          time.Duration
	Interval, Flush time.Duration
	Duration        time.Duration
}{Brokers: "127.0.0.1:39092", Prefix: "s", Lanes: 4, Streams: 64, Size: 1024, Linger: 5 * time.Millisecond, Interval: 5 * time.Second, Flush: 60 * time.Second, Duration: time.Minute}

// ProduceCommand is `loadgen produce`.
type ProduceCommand struct{}

// Usage describes the command.
func (*ProduceCommand) Usage() string {
	return "paced producer (--rate/--duration, --steps r:d,..., or --ramp from:to:step:d); JSON lines on stdout"
}

// Run parses flags, optionally creates the topic, and runs the plan.
func (*ProduceCommand) Run(ctx context.Context, args []string) error {
	cfg, out, err := ProduceFlags{}.Parse(args)
	if err != nil {
		return err
	}
	if cfg.Partitions > 0 {
		if err := (TopicPreparer{}).Ensure(ctx, cfg.Brokers, cfg.Partitions, cfg.Topics...); err != nil {
			return err
		}
	}
	w := stream.NewWriter(os.Stdout)
	p, err := loadgen.NewProducer(cfg, w)
	if err != nil {
		return err
	}
	sum := p.Run(ctx)
	w.Emit(stream.Line{Kind: stream.KindSummary, Role: stream.RoleProduce, AtNs: time.Now().UnixNano(), PSum: &sum})
	return ReportFile{}.Write(out, sum)
}

// ProduceFlags parses the command line of `loadgen produce`.
type ProduceFlags struct {
	cfg                                loadgen.ProduceConfig
	brokers, steps, ramp, keys, topics *string
	tplFile, acks, comp, out           *string
	partitions, batch                  *int
	rate                               *float64
	duration                           *time.Duration
}

// Parse returns the config and the optional summary file path.
func (ProduceFlags) Parse(args []string) (loadgen.ProduceConfig, string, error) {
	fs := flag.NewFlagSet("produce", flag.ContinueOnError)
	f := &ProduceFlags{}
	f.bindPlan(fs)
	f.bindClient(fs)
	if err := fs.Parse(args); err != nil {
		return f.cfg, "", err
	}
	return f.build()
}

func (f *ProduceFlags) bindPlan(fs *flag.FlagSet) {
	d := ProduceDefaults
	f.topics = fs.String("topic", "", "topic(s) to produce to, comma separated; stream i goes to topic i mod n (required)")
	f.partitions = fs.Int("partitions", 0, "create the topic with N partitions if missing (stress- topics only)")
	f.rate = fs.Float64("rate", 1000, "fixed target rate, events/s (0 = unlimited)")
	f.duration = fs.Duration("duration", d.Duration, "duration of the fixed-rate plan")
	f.steps = fs.String("steps", "", "step plan rate:duration,... (overrides --rate)")
	f.ramp = fs.String("ramp", "", "ramp plan from:to:step:duration (overrides --rate)")
	fs.IntVar(&f.cfg.Lanes, "lanes", d.Lanes, "producer goroutines")
	fs.IntVar(&f.cfg.Streams, "streams", d.Streams, "number of logical streams (keys); each has its own seq")
	fs.StringVar(&f.cfg.StreamPrefix, "stream-prefix", d.Prefix, "stream/key name prefix (unique per loadgen instance)")
	f.keys = fs.String("keys", string(payload.KeyRoundRobin), "key strategy: roundrobin|random|none")
	f.tplFile = fs.String("template", "", "JSON object template file (default: built-in order event)")
	fs.IntVar(&f.cfg.Size, "size", d.Size, "target event size in bytes (template padded with a pad member)")
	fs.Float64Var(&f.cfg.BadRatio, "bad-ratio", 0, "fraction of events with bad=true (route JS can throw on it)")
	f.out = fs.String("out", "", "also write the summary JSON to this file")
	fs.Bool(strings.TrimPrefix(string(SwitchStopOnEOF), "--"), false, "stop when stdin closes")
}

func (f *ProduceFlags) bindClient(fs *flag.FlagSet) {
	d := ProduceDefaults
	f.brokers = fs.String("brokers", d.Brokers, "bootstrap servers, comma separated")
	f.acks = fs.String("acks", string(loadgen.AcksAll), "acks: all|leader|none")
	fs.DurationVar(&f.cfg.Linger, "linger", d.Linger, "producer linger")
	f.batch = fs.Int("batch-bytes", 0, "max batch bytes (0 = franz-go default 1MB)")
	f.comp = fs.String("compression", string(loadgen.CodecNone), "none|lz4|zstd|snappy|gzip")
	fs.IntVar(&f.cfg.MaxBuffered, "max-buffered", 0, "max buffered records (0 = franz-go default)")
	fs.DurationVar(&f.cfg.Interval, "interval", d.Interval, "report interval")
	fs.DurationVar(&f.cfg.FlushTimeout, "flush-timeout", d.Flush, "final flush timeout")
}

func (f *ProduceFlags) build() (loadgen.ProduceConfig, string, error) {
	cfg := f.cfg
	cfg.Brokers, cfg.Partitions, cfg.Keys = strings.Split(*f.brokers, ","), int32(*f.partitions), payload.KeyMode(*f.keys)
	if *f.topics != "" {
		cfg.Topics = strings.Split(*f.topics, ",")
	}
	cfg.Acks, cfg.BatchBytes, cfg.Compression = loadgen.AcksMode(*f.acks), int32(*f.batch), loadgen.Codec(*f.comp)
	plan, err := PlanFlags{}.Resolve(*f.rate, *f.duration, *f.steps, *f.ramp)
	if err != nil {
		return cfg, "", err
	}
	cfg.Plan = plan
	cfg.Template, err = TemplateSource{}.Read(*f.tplFile)
	if len(cfg.Topics) == 0 && err == nil {
		err = errors.New("--topic is required")
	}
	if cfg.Lanes < 1 && err == nil {
		err = errors.New("--lanes must be >= 1")
	}
	return cfg, *f.out, err
}

// PlanFlags turns the plan flags into a plan (steps > ramp > fixed).
type PlanFlags struct{}

// Resolve picks the plan form given on the command line.
func (PlanFlags) Resolve(rate float64, d time.Duration, steps, ramp string) (pacing.Plan, error) {
	switch {
	case steps != "":
		return pacing.Planner{}.ParseSteps(steps)
	case ramp != "":
		return pacing.Planner{}.ParseRamp(ramp)
	default:
		return pacing.Planner{}.Fixed(rate, d), nil
	}
}

// TemplateSource loads the payload template.
type TemplateSource struct{}

// Read returns the file content, or the built-in template for an empty path.
func (TemplateSource) Read(path string) ([]byte, error) {
	if path == "" {
		return []byte(payload.DefaultTemplate), nil
	}
	return os.ReadFile(path)
}

// TopicPreparer creates the produce topic when asked.
type TopicPreparer struct{}

// Ensure creates topic with partitions if it does not exist (guarded to stress- names).
func (TopicPreparer) Ensure(ctx context.Context, brokers []string, partitions int32, topics ...string) error {
	adm, err := kafkaops.Dial(brokers)
	if err != nil {
		return err
	}
	defer adm.Close()
	return adm.EnsureTopics(ctx, partitions, topics...)
}

// ReportFile writes a summary as indented JSON.
type ReportFile struct{}

// Write writes v to path; an empty path writes nothing.
func (ReportFile) Write(path string, v any) error {
	if path == "" {
		return nil
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}
