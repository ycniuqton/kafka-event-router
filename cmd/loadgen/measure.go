package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/eventrouter/router/test/stress/kit/kafkaops"
	"github.com/eventrouter/router/test/stress/kit/loadgen"
	"github.com/eventrouter/router/test/stress/kit/stream"
)

// MeasureDefaults are the defaults of `loadgen measure`.
var MeasureDefaults = struct {
	Interval, DrainIdle, DrainMax, Poll time.Duration
}{Interval: 5 * time.Second, DrainIdle: 5 * time.Second, DrainMax: time.Minute, Poll: 500 * time.Millisecond}

// MeasureCommand is `loadgen measure`.
type MeasureCommand struct{}

// Usage describes the command.
func (*MeasureCommand) Usage() string {
	return "consume output topic(s): e2e latency p50/p95/p99/max from stress-sent-ns, duplicates/gaps per stream"
}

// Run parses flags and measures until stopped, --expect reached or --max-duration elapsed.
func (*MeasureCommand) Run(ctx context.Context, args []string) error {
	cfg, out, err := MeasureFlags{}.Parse(args)
	if err != nil {
		return err
	}
	w := stream.NewWriter(os.Stdout)
	m, err := loadgen.NewMeasurer(ctx, cfg, w)
	if err != nil {
		return err
	}
	sum := m.Run(ctx)
	w.Emit(stream.Line{Kind: stream.KindSummary, Role: stream.RoleMeasure, AtNs: time.Now().UnixNano(), MSum: &sum})
	return ReportFile{}.Write(out, sum)
}

// MeasureFlags parses the command line of `loadgen measure`.
type MeasureFlags struct{}

// Parse returns the config and the summary file path.
func (MeasureFlags) Parse(args []string) (loadgen.MeasureConfig, string, error) {
	fs := flag.NewFlagSet("measure", flag.ContinueOnError)
	d := MeasureDefaults
	var cfg loadgen.MeasureConfig
	brokers := fs.String("brokers", ProduceDefaults.Brokers, "bootstrap servers, comma separated")
	topics := fs.String("topics", "", "output topics, comma separated (required)")
	fs.BoolVar(&cfg.ReadCommitted, "read-committed", false, "read_committed isolation (exactly-once routes)")
	from := fs.String("from", string(kafkaops.FromEnd), "start offsets: end|start")
	src := fs.String("seq-source", string(loadgen.SeqFromHeader), "where stream/seq are read: header|payload")
	track := fs.String("track-by", string(loadgen.TrackStream), "delivery identity: stream|topic-stream (fan-out)")
	fs.Uint64Var(&cfg.Expect, "expect", 0, "stop once this many distinct events arrived (0 = until stopped)")
	fs.DurationVar(&cfg.MaxDuration, "max-duration", 0, "hard stop after this long (0 = none)")
	fs.DurationVar(&cfg.DrainIdle, "drain-idle", d.DrainIdle, "after a stop request, keep reading until idle this long")
	fs.DurationVar(&cfg.DrainMax, "drain-max", d.DrainMax, "after a stop request, report the summary after this long even if records still trickle in")
	fs.DurationVar(&cfg.Interval, "interval", d.Interval, "window report interval")
	fs.DurationVar(&cfg.PollTimeout, "poll-timeout", d.Poll, "max wait of one poll")
	out := fs.String("out", "", "write the summary JSON to this file")
	fs.Bool(strings.TrimPrefix(string(SwitchStopOnEOF), "--"), false, "stop (then drain) when stdin closes")
	if err := fs.Parse(args); err != nil {
		return cfg, "", err
	}
	cfg.Brokers, cfg.From, cfg.SeqSource = strings.Split(*brokers, ","), kafkaops.StartPoint(*from), loadgen.SeqSource(*src)
	cfg.TrackBy = loadgen.TrackBy(*track)
	if *topics == "" {
		return cfg, "", errors.New("--topics is required")
	}
	cfg.Topics = strings.Split(*topics, ",")
	return cfg, *out, nil
}
